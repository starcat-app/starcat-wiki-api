package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type CodeWikiProbe struct {
	client *BaseRequest
	enableRPC bool
}

func NewCodeWikiProbe(client *BaseRequest, enableRPC bool) *CodeWikiProbe {
	return &CodeWikiProbe{client: client, enableRPC: enableRPC}
}

func (p *CodeWikiProbe) Source() Source { return SourceCodeWiki }
func (p *CodeWikiProbe) Name() string   { return "Google Code Wiki" }

func (p *CodeWikiProbe) Probe(ctx context.Context, owner, repo string) ProbeResult {
	pageURL := fmt.Sprintf("https://codewiki.google/github.com/%s/%s", owner, repo)
	result := ProbeResult{
		Source:      p.Source(),
		URL:         pageURL,
		ProbeMethod: "url_probe",
	}

	if !p.enableRPC {
		return p.urlProbe(ctx, owner, repo, result)
	}

	return p.rpcProbe(ctx, owner, repo, result)
}

func (p *CodeWikiProbe) urlProbe(ctx context.Context, owner, repo string, result ProbeResult) ProbeResult {
	resp, err := p.client.Get(ctx, result.URL)
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

	if resp.StatusCode == http.StatusOK {
		result.Status = StatusUnknown
		result.Confidence = "low"
		return result
	}

	result.Status = StatusError
	return result
}

func (p *CodeWikiProbe) rpcProbe(ctx context.Context, owner, repo string, result ProbeResult) ProbeResult {
	result.ProbeMethod = "batchexecute_fetch"
	
	ghURL := fmt.Sprintf("https://github.com/%s/%s", owner, repo)
	// RPC payload: [["VSX6ub","[\"https://github.com/owner/repo\"]",null,"generic"]]
	payload := fmt.Sprintf(`[["VSX6ub","[\"%s\"]",null,"generic"]]`, ghURL)
	
	formData := url.Values{}
	formData.Set("f.req", payload)
	
	rpcURL := "https://codewiki.google/_/BoqAngularSdlcAgentsUi/data/batchexecute?rpcids=VSX6ub&rt=c&source-path=/github.com/" + owner + "/" + repo
	
	req, err := http.NewRequestWithContext(ctx, "POST", rpcURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return p.urlProbe(ctx, owner, repo, result)
	}
	
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", p.client.userAgent)
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return p.urlProbe(ctx, owner, repo, result)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return p.urlProbe(ctx, owner, repo, result)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return p.urlProbe(ctx, owner, repo, result)
	}
	
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "wrb.fr") || !strings.Contains(bodyStr, "VSX6ub") {
		return p.urlProbe(ctx, owner, repo, result)
	}
	
	// 宽松匹配
	signals := []string{"rpc_ok"}
	if strings.Contains(bodyStr, "canonicalUrl") && strings.Contains(bodyStr, ghURL) {
		signals = append(signals, "canonical_url_matched")
	}
	
	if strings.Contains(bodyStr, "markdown") || strings.Contains(bodyStr, "sections") {
		signals = append(signals, "sections_found")
	}
	
	result.MatchedSignals = signals
	
	if len(signals) >= 3 {
		result.Status = StatusIndexed
		result.Confidence = "high"
	} else if len(signals) >= 2 {
		result.Status = StatusProbablyIndexed
		result.Confidence = "medium"
	} else {
		result.Status = StatusUnknown
		result.Confidence = "low"
	}
	
	return result
}
