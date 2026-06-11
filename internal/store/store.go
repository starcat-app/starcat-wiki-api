package store

import (
	"context"

	"github.com/dong4j/starcat-wiki-api/internal/probe"
)

// Store 数据存储接口。
type Store interface {
	// GetProbes 获取某个 repo 的全部探测结果（不包含 attempt 等内部字段）。
	GetProbes(ctx context.Context, owner, repo string) ([]probe.ProbeResult, error)

	// UpsertProbe 写入/更新单条探测结果（成功结果重置 attempt）。
	UpsertProbe(ctx context.Context, owner, repo string, result probe.ProbeResult) error

	// GetExpiredProbes 获取已过期的探测记录（用于定时刷新）。
	GetExpiredProbes(ctx context.Context, limit int) ([]ProbeRecord, error)

	// --- v2 新增 ---

	// InsertPendingProbes 为指定 repo 预插入 3 条 probing 记录（每个 source 一条）。
	// INSERT OR IGNORE，已存在的不覆盖。
	InsertPendingProbes(ctx context.Context, owner, repo string) error

	// GetPendingProbes 获取所有 status='probing' 的记录（宕机恢复用）。
	GetPendingProbes(ctx context.Context) ([]ProbeRecord, error)

	// GetRetryableErrors 获取待重试的 error 记录（含 attempt 信息）。
	// 条件：status='error' AND attempt < maxAttempts AND next_retry_at <= now。
	GetRetryableErrors(ctx context.Context, maxAttempts int) ([]ProbeRecordWithAttempt, error)

	// UpdateProbeResult 更新单条探测结果（probing/error → final）。
	// 成功结果：attempt 重置为 0，last_error/next_retry_at 清空。
	// 失败结果：保留 attempt/last_error/next_retry_at。
	UpdateProbeResult(ctx context.Context, owner, repo string, source probe.Source,
		result probe.ProbeResult, attempt int, lastError string, nextRetryAt string) error

	// IncrementAndResetProbing 原子递增 attempt 并将 error 记录重置为 probing。
	IncrementAndResetProbing(ctx context.Context, owner, repo string,
		source probe.Source, nextRetryAt string) error

	// MarkNotIndexed 将指定记录强制标记为 not_indexed（反爬/放弃重试）。
	MarkNotIndexed(ctx context.Context, owner, repo string, source probe.Source) error

	// AbandonMaxAttempts 将 attempt >= maxAttempts 的 error 记录标记为 not_indexed。
	// 返回被放弃的记录数。
	AbandonMaxAttempts(ctx context.Context, maxAttempts int) (int, error)

	Close() error
}

// ProbeRecord 探测记录的定位信息。
type ProbeRecord struct {
	Owner  string
	Repo   string
	Source probe.Source
}

// ProbeRecordWithAttempt 含 attempt 信息的探测记录（重试查询用）。
type ProbeRecordWithAttempt struct {
	Owner   string
	Repo    string
	Source  probe.Source
	Attempt int
}
