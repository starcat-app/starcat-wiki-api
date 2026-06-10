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

	"golang.org/x/sync/semaphore"

	"github.com/dong4j/starcat-wiki-api/internal/model"
	"github.com/dong4j/starcat-wiki-api/internal/probe"
	"github.com/dong4j/starcat-wiki-api/internal/store"
)

type ProbeHandler struct {
	store    store.Store
	probes   map[probe.Source]probe.Probe
	inflight sync.Map
	sem      *semaphore.Weighted
	
	batchMinDelay int
	batchMaxDelay int
}

func NewProbeHandler(s store.Store, p map[probe.Source]probe.Probe) *ProbeHandler {
	concurrency := 4
	if c, err := strconv.Atoi(os.Getenv("PROBE_CONCURRENCY")); err == nil && c > 0 {
		concurrency = c
	}

	minDelay := 80
	if c, err := strconv.Atoi(os.Getenv("PROBE_BATCH_MIN_DELAY_MS")); err == nil && c > 0 {
		minDelay = c
	}

	maxDelay := 400
	if c, err := strconv.Atoi(os.Getenv("PROBE_BATCH_MAX_DELAY_MS")); err == nil && c > 0 {
		maxDelay = c
	}

	return &ProbeHandler{
		store:         s,
		probes:        p,
		sem:           semaphore.NewWeighted(int64(concurrency)),
		batchMinDelay: minDelay,
		batchMaxDelay: maxDelay,
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
	now := time.Now()
	
	isStale := false
	for _, res := range results {
		if res.ExpiresAt != nil && now.After(*res.ExpiresAt) {
			isStale = true
			break
		}
	}

	if len(results) == 0 {
		// Cold start: 同步探测
		results = h.syncProbe(r.Context(), owner, repo)
		cacheStatus = "cold"
	} else if len(results) < len(h.probes) || isStale {
		// 部分缺失或已过期 -> stale, 后台异步刷新
		cacheStatus = "stale"
		go h.asyncRefresh(owner, repo)
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

	if len(req.Repos) == 0 {
		writeJSON(w, map[string]interface{}{"results": map[string][]probe.ProbeResult{}})
		return
	}
	if len(req.Repos) > 50 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "too many repos (max 50)", nil)
		return
	}

	allResults := make(map[string][]probe.ProbeResult)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, fullName := range req.Repos {
		parts := strings.SplitN(fullName, "/", 2)
		if len(parts) != 2 {
			continue
		}
		owner, repo := parts[0], parts[1]

		results, _ := h.store.GetProbes(r.Context(), owner, repo)
		isStale := len(results) < len(h.probes)
		now := time.Now()
		for _, res := range results {
			if res.ExpiresAt != nil && now.After(*res.ExpiresAt) {
				isStale = true
				break
			}
		}

		if len(results) > 0 {
			allResults[fullName] = results
		}

		if len(results) == 0 || isStale {
			// 需要后台排队探测
			wg.Add(1)
			go func(o, r string) {
				defer wg.Done()
				_ = h.sem.Acquire(context.Background(), 1)
				defer h.sem.Release(1)

				probe.RandomDelay(h.batchMinDelay, h.batchMaxDelay)
				res := h.syncProbe(context.Background(), o, r)
				
				mu.Lock()
				allResults[o+"/"+r] = res
				mu.Unlock()
			}(owner, repo)
		}
	}
	
	wg.Wait()
	writeJSONWithMeta(w, map[string]interface{}{"results": allResults}, &model.Meta{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Total:       len(allResults),
	})
}

func (h *ProbeHandler) syncProbe(ctx context.Context, owner, repo string) []probe.ProbeResult {
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := []probe.ProbeResult{}

	for _, p := range h.probes {
		wg.Add(1)
		go func(p probe.Probe) {
			defer wg.Done()
			
			// 并发控制
			_ = h.sem.Acquire(context.Background(), 1)
			res := p.Probe(ctx, owner, repo)
			h.sem.Release(1)
			
			mu.Lock()
			results = append(results, res)
			mu.Unlock()
			
			// 同步写库防止后续查不到
			if err := h.store.UpsertProbe(context.Background(), owner, repo, res); err != nil {
				log.Printf("[handler] syncProbe upsert error: %v", err)
			}
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

	_ = h.syncProbe(context.Background(), owner, repo)
}

