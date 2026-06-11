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
	apiURL := fmt.Sprintf("https://api.devin.ai/ada/public_repo_indexing_status?repo_name=%s/%s", owner, repo)
	pageURL := fmt.Sprintf("https://deepwiki.com/%s/%s", owner, repo)

	result := ProbeResult{
		Source:      p.Source(),
		URL:         pageURL,
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
		result.Error = fmt.Sprintf("http_%d", resp.StatusCode)
		return result
	}

	var data struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		result.Status = StatusError
		result.Error = fmt.Sprintf("json_decode_error: %v", err)
		return result
	}

	if data.Status == "completed" {
		result.Status = StatusIndexed
		result.MatchedSignals = []string{"api_status_completed"}
	} else {
		result.Status = StatusNotIndexed
		result.MatchedSignals = []string{"api_status_" + data.Status}
	}

	return result
}
