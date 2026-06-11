// Package handler 启动恢复：重新入队中断的 probing 任务。
package handler

import (
	"context"
	"log"
)

// RecoverPendingProbes 启动时恢复所有 status='probing' 的未完成任务。
//
// 适用场景：进程崩溃 / 被 kill 后重启，探测任务只入库了 probing 但未执行。
// 恢复策略：查出所有 probing 记录，重新启动异步探测。
func (h *ProbeHandler) RecoverPendingProbes() {
	records, err := h.store.GetPendingProbes(context.Background())
	if err != nil {
		log.Printf("[recover] GetPendingProbes error: %v", err)
		return
	}

	if len(records) == 0 {
		log.Println("[recover] 没有未完成的探测任务")
		return
	}

	// 按 repo 去重
	repoMap := make(map[string]bool)
	for _, r := range records {
		repoMap[r.Owner+"/"+r.Repo] = true
	}

	var repos []string
	for fullName := range repoMap {
		repos = append(repos, fullName)
	}

	log.Printf("[recover] 发现 %d 条 probing 记录（%d 个唯一 repo），重新入队",
		len(records), len(repos))

	go h.batchProbeAsync(repos)
}
