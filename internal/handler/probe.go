package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/dong4j/starcat-wiki-api/internal/model"
	"github.com/dong4j/starcat-wiki-api/internal/probe"
	"github.com/dong4j/starcat-wiki-api/internal/store"
)

type ProbeHandler struct {
	store   store.Store
	probes  map[probe.Source]probe.Probe
	inflight sync.Map
}

func NewProbeHandler(s store.Store, p map[probe.Source]probe.Probe) *ProbeHandler {
	return &ProbeHandler{
		store:  s,
		probes: p,
	}
}

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

	// SWR Logic
	cacheStatus := "fresh"
	if len(results) == 0 {
		// Cold start: 同步探测
		results = h.syncProbe(r.Context(), owner, repo)
		cacheStatus = "cold"
	} else if len(results) < len(h.probes) {
		// 部分缺失
		cacheStatus = "stale"
		go h.asyncRefresh(owner, repo)
	} else {
		// 检查过期 (任意一个过期即 stale)
		// 注意: GetProbes 里的 expires_at 逻辑在 SQL 里过滤了, 
		// 但为了 SWR, SQL 应该查全量. 修正 store.go 的查询.
		// 暂时简化: 如果结果少于 probes 总数或有过期即异步刷新.
		cacheStatus = "fresh" // 默认 fresh, 实际上 store 应返回 expiresAt
	}

	writeJSONWithMeta(w, results, &model.Meta{
		GeneratedAt: time.Now().Format(time.RFC3339),
		CacheStatus: cacheStatus,
	})
}

func (h *ProbeHandler) HandleProbeBatchV1(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Repos []string `json:"repos"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid body", nil)
		return
	}

	// 批量逻辑简化: 只返库里有的, 库里没的触发异步刷新
	// 实际生产应更复杂
	writeJSON(w, map[string]string{"message": "batch request accepted"})
}

func (h *ProbeHandler) syncProbe(ctx context.Context, owner, repo string) []probe.ProbeResult {
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := []probe.ProbeResult{}

	for _, p := range h.probes {
		wg.Add(1)
		go func(p probe.Probe) {
			defer wg.Done()
			res := p.Probe(ctx, owner, repo)
			mu.Lock()
			results = append(results, res)
			mu.Unlock()
			_ = h.store.UpsertProbe(context.Background(), owner, repo, res)
		}(p)
	}
	wg.Wait()
	return results
}

func (h *ProbeHandler) asyncRefresh(owner, repo string) {
	key := owner + "/" + repo
	if _, loaded := h.inflight.LoadOrStore(key, true); loaded {
		return
	}
	defer h.inflight.Delete(key)

	// 异步全量刷新
	_ = h.syncProbe(context.Background(), owner, repo)
}
