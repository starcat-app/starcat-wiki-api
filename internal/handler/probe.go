// Package handler 提供 wiki 探测的 HTTP 处理器。
//
// v2 核心变更：
//   - Batch 接口先预入库 probing 再异步探测，避免超时
//   - 3 个独立 per-source worker，各自按间隔限速，并行探测
//   - 错误分类 + 重试机制（network/timeout → 重试，429 → 长间隔重试，403 → 放弃）
//   - 启动时恢复 probing 中的任务
package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dong4j/starcat-wiki-api/internal/model"
	"github.com/dong4j/starcat-wiki-api/internal/probe"
	"github.com/dong4j/starcat-wiki-api/internal/store"
)

// ProbeHandler wiki 探测 HTTP 处理器。
type ProbeHandler struct {
	store  store.Store
	probes map[probe.Source]probe.Probe

	// inflight 防止同一 repo 的异步刷新重复触发。
	inflight sync.Map

	// per-source 请求间隔（毫秒）。
	sourceInterval map[probe.Source]int

	// 重试配置
	retryMaxAttempts int
	retryInterval    time.Duration
}

// NewProbeHandler 创建 ProbeHandler。
func NewProbeHandler(s store.Store, probes map[probe.Source]probe.Probe) *ProbeHandler {
	return &ProbeHandler{
		store:  s,
		probes: probes,

		sourceInterval: map[probe.Source]int{
			probe.SourceZread:    getEnvInt("PROBE_ZREAD_INTERVAL_MS", 1000),
			probe.SourceDeepWiki: getEnvInt("PROBE_DEEPWIKI_INTERVAL_MS", 1000),
			probe.SourceCodeWiki: getEnvInt("PROBE_CODEWIKI_INTERVAL_MS", 1000),
		},

		retryMaxAttempts: getEnvInt("RETRY_MAX_ATTEMPTS", 3),
		retryInterval:    time.Duration(getEnvInt("RETRY_INTERVAL_MINUTES", 30)) * time.Minute,
	}
}

func getEnvInt(key string, def int) int {
	if s := os.Getenv(key); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			return v
		}
	}
	return def
}

// HandleProbeV1 单 repo 探测（GET /api/v1/wikis?owner=X&repo=Y）。
//
// probing 状态对外映射为 not_indexed，内部保持 probing 不变。
func (h *ProbeHandler) HandleProbeV1(w http.ResponseWriter, r *http.Request) {
	owner := r.URL.Query().Get("owner")
	repo := r.URL.Query().Get("repo")

	if owner == "" || repo == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "owner and repo are required", nil)
		return
	}

	results, err := h.store.GetProbes(r.Context(), owner, repo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	cacheStatus := "fresh"
	now := time.Now()

	// 检查是否有 probing 或过期记录
	hasProbing := false
	isStale := false
	for _, res := range results {
		if res.Status == probe.StatusProbing {
			hasProbing = true
			isStale = true // probing 视为过期，等待探测完成
		}
		if res.ExpiresAt != nil && now.After(*res.ExpiresAt) {
			isStale = true
		}
	}

	if len(results) == 0 {
		// Cold start：同步探测
		results = h.syncProbe(r.Context(), owner, repo)
		cacheStatus = "cold"
	} else if isStale && !hasProbing {
		// 过期但没人在探测 → 后台刷新
		cacheStatus = "stale"
		go h.asyncRefresh(owner, repo)
	}
	// hasProbing → 不触发 asyncRefresh，等探测完成

	// probing 对外映射为 not_indexed
	externalResults := h.mapProbingToNotIndexed(results)

	writeJSONWithMeta(w, externalResults, &model.Meta{
		GeneratedAt: time.Now().Format(time.RFC3339),
		CacheStatus: cacheStatus,
	})
}

// HandleProbeBatchV1 批量探测（POST /api/v1/wikis/batch）。
//
// Phase 1（同步）：预入库 probing 记录，返回已有缓存
// Phase 2（异步）：启动 3 个 per-source worker 并行探测
func (h *ProbeHandler) HandleProbeBatchV1(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Repos []string `json:"repos"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid body", nil)
		return
	}

	if len(req.Repos) == 0 {
		writeJSON(w, map[string]interface{}{"results": map[string][]probe.ProbeResult{}})
		return
	}
	if len(req.Repos) > 50 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "too many repos (max 50)", nil)
		return
	}

	// Phase 1: 预入库 + 收集已有缓存
	cachedResults := make(map[string][]probe.ProbeResult)
	var needProbe []string
	newProbes := 0

	for _, fullName := range req.Repos {
		// 清洗前导/后置斜杠（数据源可能传入 "/owner/repo"）
		cleaned := strings.Trim(fullName, "/")
		parts := strings.SplitN(cleaned, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			log.Printf("[probe] batch: skip malformed repo name: %q", fullName)
			continue
		}
		owner, repo := parts[0], parts[1]

		results, _ := h.store.GetProbes(r.Context(), owner, repo)

		// 判断缓存新鲜度
		hasFresh := true
		hasAny := len(results) > 0
		now := time.Now()
		for _, res := range results {
			if res.Status == probe.StatusProbing {
				hasFresh = false
			}
			if res.ExpiresAt != nil && now.After(*res.ExpiresAt) {
				hasFresh = false
			}
		}
		if len(results) < len(h.probes) {
			hasFresh = false
		}

		if hasFresh {
			// 全部新鲜 → 直接返回缓存
			cachedResults[fullName] = results
		} else {
			// 需要探测 → 预入库 probing（INSERT OR IGNORE 保证不覆盖已有记录）
			if err := h.store.InsertPendingProbes(r.Context(), owner, repo); err != nil {
				log.Printf("[probe] InsertPendingProbes %s/%s error: %v", owner, repo, err)
			} else {
				newProbes++
			}

			if hasAny {
				// 有过期缓存 → 返回过期缓存 + 排队探测
				cachedResults[fullName] = results
			}
			needProbe = append(needProbe, fullName)
		}
	}

	// Phase 2: 异步探测
	if len(needProbe) > 0 {
		go h.batchProbeAsync(needProbe)
	}

	writeJSONWithMeta(w, map[string]interface{}{
		"results":    cachedResults,
		"new_probes": newProbes,
	}, &model.Meta{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Total:       len(cachedResults),
	})
}

// batchProbeAsync 启动 3 个独立的 per-source worker 并行探测。
//
// 每个 worker 只处理自己 source 的探测，按配置的间隔限速。
// 3 个 worker 之间互不等待。
func (h *ProbeHandler) batchProbeAsync(repos []string) {
	log.Printf("[probe] 开始异步探测 %d repos（3 source 并行）", len(repos))

	var wg sync.WaitGroup

	for _, src := range []probe.Source{probe.SourceZread, probe.SourceDeepWiki, probe.SourceCodeWiki} {
		p, ok := h.probes[src]
		if !ok {
			continue
		}
		interval := h.sourceInterval[src]

		wg.Add(1)
		go func(source probe.Source, pr probe.Probe, intervalMs int) {
			defer wg.Done()
			h.probeBatchForSource(source, pr, repos, intervalMs)
		}(src, p, interval)
	}

	wg.Wait()
	log.Printf("[probe] 异步探测完成: %d repos", len(repos))
}

// probeBatchForSource 对一批 repo 按指定 source 逐条探测。
// 每探测一条后 sleep intervalMs 毫秒，控制对同一 wiki 站的请求频率。
func (h *ProbeHandler) probeBatchForSource(
	src probe.Source, p probe.Probe, repos []string, intervalMs int,
) {
	sourceName := string(src)
	completed := 0

	for i, fullName := range repos {
		cleaned := strings.Trim(fullName, "/")
		parts := strings.SplitN(cleaned, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		owner, repo := parts[0], parts[1]

		// 执行探测
		result := p.Probe(context.Background(), owner, repo)

		// 根据结果分类处理
		h.handleProbeResult(context.Background(), owner, repo, src, result)

		completed++
		log.Printf("[probe] %s %s/%s → %s (%d/%d)",
			sourceName, owner, repo, result.Status, completed, len(repos))

		// 同 source 间隔控制（最后一条不 sleep）
		if i < len(repos)-1 && intervalMs > 0 {
			time.Sleep(time.Duration(intervalMs) * time.Millisecond)
		}
	}

	log.Printf("[probe] %s worker 完成: %d/%d repos", sourceName, completed, len(repos))
}

// handleProbeResult 处理单条探测结果：根据状态和错误分类决定写入策略。
func (h *ProbeHandler) handleProbeResult(
	ctx context.Context, owner, repo string, src probe.Source, result probe.ProbeResult,
) {
	switch result.Status {
	case probe.StatusIndexed, probe.StatusNotIndexed:
		// 成功确定结果 → 直接写入
		if err := h.store.UpdateProbeResult(ctx, owner, repo, src, result,
			0, "", ""); err != nil {
			log.Printf("[probe] UpdateProbeResult %s/%s %s error: %v",
				owner, repo, src, err)
		}

	case probe.StatusError:
		// 错误 → 分类决定重试策略
		category, retryable := probe.ClassifyError(result)

		if !retryable {
			// 403 反爬 → 直接标记 not_indexed，不再重试
			log.Printf("[probe] %s/%s %s → anti_crawl/403，标记 not_indexed: %s",
				owner, repo, src, result.Error)
			if err := h.store.MarkNotIndexed(ctx, owner, repo, src); err != nil {
				log.Printf("[probe] MarkNotIndexed %s/%s %s error: %v",
					owner, repo, src, err)
			}
			return
		}

			// 可重试错误 → 设置 attempt=1 + last_error + next_retry_at
			// 后续重试由定时任务的 IncrementAndResetProbing 原子递增 attempt


		retryInterval := h.retryInterval
		if category == probe.ErrRateLimit {
			// 429 → 更长间隔（retry_interval × 4）
			retryInterval = h.retryInterval * 4
		}

		nextRetryAt := time.Now().Add(retryInterval).Format(time.RFC3339)

		if err := h.store.UpdateProbeResult(ctx, owner, repo, src, result,
			1, // 首次标记 error，定时任务会递增
			result.Error, nextRetryAt); err != nil {
			log.Printf("[probe] UpdateProbeResult(error) %s/%s %s: %v",
				owner, repo, src, err)
		}

		log.Printf("[probe] %s/%s %s → error(%s), 下次重试: %s",
			owner, repo, src, category, nextRetryAt)
	}
}

// syncProbe 同步探测一个 repo 的所有 source（用于冷启动 GET）。
func (h *ProbeHandler) syncProbe(ctx context.Context, owner, repo string) []probe.ProbeResult {
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := []probe.ProbeResult{}

	for _, p := range h.probes {
		wg.Add(1)
		go func(pr probe.Probe) {
			defer wg.Done()

			res := pr.Probe(ctx, owner, repo)

			mu.Lock()
			results = append(results, res)
			mu.Unlock()

			// 同步写库
			if err := h.store.UpsertProbe(context.Background(), owner, repo, res); err != nil {
				log.Printf("[probe] syncProbe upsert %s/%s %s: %v",
					owner, repo, pr.Source(), err)
			}
		}(p)
	}
	wg.Wait()
	return results
}

// asyncRefresh 后台异步刷新过期缓存。
// 如果 DB 已有 probing 记录则不重复探测。
func (h *ProbeHandler) asyncRefresh(owner, repo string) {
	key := owner + "/" + repo
	if _, loaded := h.inflight.LoadOrStore(key, true); loaded {
		return
	}
	defer h.inflight.Delete(key)

	// 检查是否已有 probing 记录（避免重复入队）
	existing, _ := h.store.GetProbes(context.Background(), owner, repo)
	for _, r := range existing {
		if r.Status == probe.StatusProbing {
			return // 已有 probing，不重复探测
		}
	}

	// 预入库 probing
	if err := h.store.InsertPendingProbes(context.Background(), owner, repo); err != nil {
		log.Printf("[probe] asyncRefresh InsertPendingProbes %s/%s: %v", owner, repo, err)
	}

	// 异步探测所有 source
	go h.batchProbeAsync([]string{owner + "/" + repo})
}

// mapProbingToNotIndexed 将 probing 状态映射为 not_indexed 返回给客户端。
// 客户端不需要感知内部 probing 状态。
func (h *ProbeHandler) mapProbingToNotIndexed(results []probe.ProbeResult) []probe.ProbeResult {
	mapped := make([]probe.ProbeResult, len(results))
	for i, r := range results {
		mapped[i] = r
		if r.Status == probe.StatusProbing {
			mapped[i].Status = probe.StatusNotIndexed
		}
	}
	return mapped
}
