package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	url := fmt.Sprintf("https://zread.ai/%s/%s", owner, repo)
	result := ProbeResult{
		Source:      p.Source(),
		URL:         url,
		ProbeMethod: "html_fingerprint",
	}

	resp, err := p.client.Get(ctx, url)
	if err != nil {
		result.Status = StatusError
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	result.HTTPStatus = &resp.StatusCode

	if resp.StatusCode == http.StatusNotFound {
		result.Status = StatusNotIndexed
		result.Confidence = "high"
		return result
	}

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		result.Status = StatusRateLimited
		result.Confidence = "high"
		return result
	}

	if resp.StatusCode != http.StatusOK {
		result.Status = StatusError
		return result
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		result.Status = StatusUnknown
		return result
	}

	bodyStr := string(body)
	backlink := fmt.Sprintf("github.com/%s/%s", owner, repo)
	signals := []string{}

	if strings.Contains(bodyStr, backlink) {
		signals = append(signals, "backlink")
	}

	lowerBody := strings.ToLower(bodyStr)
	keywords := []string{"ask ai", "source code", "overview"}
	for _, k := range keywords {
		if strings.Contains(lowerBody, k) {
			signals = append(signals, strings.ReplaceAll(k, " ", "_"))
		}
	}

	result.MatchedSignals = signals

	if len(signals) >= 3 {
		result.Status = StatusIndexed
		result.Confidence = "high"
	} else if len(signals) >= 1 {
		result.Status = StatusProbablyIndexed
		result.Confidence = "medium"
	} else {
		result.Status = StatusNotIndexed
		result.Confidence = "high"
	}

	return result
}
