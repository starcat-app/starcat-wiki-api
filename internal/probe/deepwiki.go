package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type DeepWikiProbe struct {
	client *BaseRequest
}

func NewDeepWikiProbe(client *BaseRequest) *DeepWikiProbe {
	return &DeepWikiProbe{client: client}
}

func (p *DeepWikiProbe) Source() Source { return SourceDeepWiki }
func (p *DeepWikiProbe) Name() string   { return "DeepWiki" }

func (p *DeepWikiProbe) Probe(ctx context.Context, owner, repo string) ProbeResult {
	// API 端点: https://api.devin.ai/ada/public_repo_indexing_status?repo_name=owner/repo
	apiURL := fmt.Sprintf("https://api.devin.ai/ada/public_repo_indexing_status?repo_name=%s/%s", owner, repo)
	pageURL := fmt.Sprintf("https://deepwiki.com/%s/%s", owner, repo)

	result := ProbeResult{
		Source:      p.Source(),
		URL:         pageURL, // 结果展示仍用展示页 URL
		ProbeMethod: "json_api",
	}

	resp, err := p.client.Get(ctx, apiURL)
	if err != nil {
		result.Status = StatusError
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	result.HTTPStatus = &resp.StatusCode

	if resp.StatusCode != http.StatusOK {
		result.Status = StatusError
		return result
	}

	var data struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		result.Status = StatusUnknown
		result.Error = "json_decode_error"
		return result
	}

	if data.Status == "completed" {
		result.Status = StatusIndexed
		result.Confidence = "high"
		result.MatchedSignals = []string{"api_status_completed"}
	} else {
		result.Status = StatusNotIndexed
		result.Confidence = "high"
		result.MatchedSignals = []string{"api_status_" + data.Status}
	}

	return result
}
