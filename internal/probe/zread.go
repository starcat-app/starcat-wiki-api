package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type ZreadProbe struct {
	client *BaseRequest
}

func NewZreadProbe(client *BaseRequest) *ZreadProbe {
	return &ZreadProbe{client: client}
}

func (p *ZreadProbe) Source() Source { return SourceZread }
func (p *ZreadProbe) Name() string   { return "Zread" }

func (p *ZreadProbe) Probe(ctx context.Context, owner, repo string) ProbeResult {
	apiURL := fmt.Sprintf("https://zread.ai/api/v1/repo/github/%s/%s", owner, repo)
	pageURL := fmt.Sprintf("https://zread.ai/%s/%s", owner, repo)

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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Status = StatusError
		result.Error = fmt.Sprintf("read_body_error: %v", err)
		return result
	}

	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		snippet := string(body)
		if len(snippet) > 100 {
			snippet = snippet[:100]
		}
		result.Status = StatusError
		result.Error = fmt.Sprintf("json_decode_error: %s", snippet)
		return result
	}

	if envelope.Data.Status == "success" {
		result.Status = StatusIndexed
		result.MatchedSignals = []string{"api_status_success"}
	} else {
		result.Status = StatusNotIndexed
		result.MatchedSignals = []string{"api_status_" + envelope.Data.Status}
	}

	return result
}
