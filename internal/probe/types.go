package probe

import (
	"context"
)

type Source string

const (
	SourceDeepWiki Source = "deepwiki"
	SourceCodeWiki Source = "codewiki"
	SourceZread    Source = "zread"
)

type Status string

const (
	StatusIndexed         Status = "indexed"
	StatusProbablyIndexed Status = "probably_indexed"
	StatusNotIndexed      Status = "not_indexed"
	StatusUnknown         Status = "unknown"
	StatusError           Status = "error"
	StatusRateLimited     Status = "rate_limited"
)

type ProbeResult struct {
	Source         Source   `json:"source"`
	Status         Status   `json:"status"`
	URL            string   `json:"url"`
	Confidence     string   `json:"confidence"`
	ProbeMethod    string   `json:"probeMethod"`
	HTTPStatus     *int     `json:"httpStatus,omitempty"`
	MatchedSignals []string `json:"matchedSignals,omitempty"`
	Error          string   `json:"error,omitempty"`
}

type Probe interface {
	Source() Source
	Name() string
	Probe(ctx context.Context, owner, repo string) ProbeResult
}
