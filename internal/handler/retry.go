// Package handler 定时重试 error 状态的探测记录。
//
// 重试策略：
//   - 每 RETRY_INTERVAL_MINUTES 分钟执行一次（默认 30）
//   - 查询 status='error' AND attempt < RETRY_MAX_ATTEMPTS AND next_retry_at <= now
//   - 原子递增 attempt + 重置为 probing 并重新入队
//   - attempt >= RETRY_MAX_ATTEMPTS 的自动标记为 not_indexed（放弃重试）
package handler

import (
	"context"
	"log"
	"time"
)

// RetryErrors 定时重试 error 状态的记录。
//
// 应由 cron 周期调用。内部流程：
//  1. 放弃 attempt >= max 的记录 → not_indexed
//  2. 获取可重试的记录
//  3. 原子递增 attempt + 重置为 probing
//  4. 重新入队异步探测
func (h *ProbeHandler) RetryErrors() {
	ctx := context.Background()

	// 1. 放弃已达最大重试次数的记录
	abandoned, err := h.store.AbandonMaxAttempts(ctx, h.retryMaxAttempts)
	if err != nil {
		log.Printf("[retry] AbandonMaxAttempts error: %v", err)
	} else if abandoned > 0 {
		log.Printf("[retry] 放弃 %d 条已达最大重试次数(%d)的记录 → not_indexed",
			abandoned, h.retryMaxAttempts)
	}

	// 2. 获取可重试的记录
	records, err := h.store.GetRetryableErrors(ctx, h.retryMaxAttempts)
	if err != nil {
		log.Printf("[retry] GetRetryableErrors error: %v", err)
		return
	}

	if len(records) == 0 {
		return
	}

	// 3. 原子递增 attempt + 重置为 probing
	nextRetry := time.Now().Add(h.retryInterval).Format(time.RFC3339)
	repoMap := make(map[string]bool)

	for _, r := range records {
		if err := h.store.IncrementAndResetProbing(ctx, r.Owner, r.Repo, r.Source, nextRetry); err != nil {
			log.Printf("[retry] IncrementAndResetProbing %s/%s %s error: %v",
				r.Owner, r.Repo, r.Source, err)
			continue
		}
		repoMap[r.Owner+"/"+r.Repo] = true
	}

	if len(repoMap) == 0 {
		return
	}

	var repos []string
	for fullName := range repoMap {
		repos = append(repos, fullName)
	}

	log.Printf("[retry] 重新入队 %d 条（%d repo），下次重试: %s",
		len(records), len(repos), nextRetry)

	// 4. 启动异步探测
	go h.batchProbeAsync(repos)
}
