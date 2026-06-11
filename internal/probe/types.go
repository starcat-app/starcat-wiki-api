// Package probe 定义 wiki 探测核心类型和接口。
//
// 状态简化（v2）：
//   probing     — 已入库但尚未探测（批次预占位）
//   indexed     — 已确认该 wiki 收录此仓库
//   not_indexed — 已确认该 wiki 未收录此仓库（或放弃重试后的终态）
//   error       — 探测失败，等待重试（attempt < max）
//
// 旧状态映射：
//   probably_indexed → indexed
//   unknown          → error（临时失败，可重试）或 not_indexed（JSON 解析失败直接判定）
//   rate_limited     → error（429 可重试，403 不可重试 → not_indexed）
package probe

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Source 标识 wiki 来源。
type Source string

const (
	SourceDeepWiki Source = "deepwiki"
	SourceCodeWiki Source = "codewiki"
	SourceZread    Source = "zread"
)

// Status 探测状态。
type Status string

const (
	StatusProbing    Status = "probing"
	StatusIndexed    Status = "indexed"
	StatusNotIndexed Status = "not_indexed"
	StatusError      Status = "error"
)

// ErrorCategory 错误分类，决定重试策略。
type ErrorCategory string

const (
	ErrNetwork   ErrorCategory = "network_error"   // 网络问题，可重试
	ErrRateLimit ErrorCategory = "rate_limited"     // 429，更长间隔重试
	ErrAntiCrawl ErrorCategory = "anti_crawl"       // 403，不可重试→not_indexed
	ErrProbe     ErrorCategory = "probe_error"      // API 返回异常，可重试
)

// ProbeResult 单次探测结果。
type ProbeResult struct {
	Source         Source     `json:"source"`
	Status         Status     `json:"status"`
	URL            string     `json:"url"`
	ProbeMethod    string     `json:"probeMethod"`
	HTTPStatus     *int       `json:"httpStatus,omitempty"`
	MatchedSignals []string   `json:"matchedSignals,omitempty"`
	Error          string     `json:"error,omitempty"`
	ExpiresAt      *time.Time `json:"-"` // 内部使用，不序列化到客户端
}

// ClassifyError 根据 ProbeResult 分类错误，返回类别和是否可重试。
//
// 分类规则：
//   - HTTP 403 → anti_crawl，不可重试
//   - HTTP 429 → rate_limited，可重试（更长间隔）
//   - 其他 HTTP 4xx/5xx → network_error，可重试
//   - 网络错误（timeout/DNS/connection refused）→ network_error，可重试
//   - JSON 解析失败等 → probe_error，可重试
func ClassifyError(result ProbeResult) (ErrorCategory, bool) {
	httpStatus := 0
	if result.HTTPStatus != nil {
		httpStatus = *result.HTTPStatus
	}

	// 403 → 反爬，不可重试
	if httpStatus == http.StatusForbidden {
		return ErrAntiCrawl, false
	}

	// 429 → 限流，可重试但需要更长间隔
	if httpStatus == http.StatusTooManyRequests {
		return ErrRateLimit, true
	}

	errLower := strings.ToLower(result.Error)

	// 网络层错误特征
	networkKeywords := []string{
		"timeout", "connection refused", "connection reset",
		"no such host", "eof", "tls", "dial tcp", "deadline exceeded",
	}
	for _, kw := range networkKeywords {
		if strings.Contains(errLower, kw) {
			return ErrNetwork, true
		}
	}

	// 非网络错误但也不是明确的"未索引"→ probe_error，可重试
	if result.Error != "" {
		return ErrProbe, true
	}

	return ErrNetwork, true // 兜底：可重试
}

// Probe 探测接口，每个 wiki 源实现此接口。
type Probe interface {
	Source() Source
	Name() string
	Probe(ctx context.Context, owner, repo string) ProbeResult
}
