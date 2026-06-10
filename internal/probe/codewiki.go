package probe

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
)

type CodeWikiProbe struct {
	client    *BaseRequest
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
	fullName := fmt.Sprintf("%s/%s", owner, repo)

	// source-path 必须 URL 编码：/github.com/{owner}/{repo} → %2Fgithub.com%2F{owner}%2F{repo}
	encodedPath := url.QueryEscape(fmt.Sprintf("/github.com/%s/%s", owner, repo))

	// f.req 载荷：batchexecute RPC 格式，3 层括号，传入完整 GitHub URL
	payload := fmt.Sprintf(`[[["VSX6ub","[\"%s\"]",null,"generic"]]]`, ghURL)
	formData := url.Values{}
	formData.Set("f.req", payload)

	// 构造 RPC URL：source-path 已编码，rt=c 表示紧凑响应
	rpcURL := fmt.Sprintf(
		"https://codewiki.google/_/BoqAngularSdlcAgentsUi/data/batchexecute?rpcids=VSX6ub&source-path=%s&rt=c&_reqid=%d",
		encodedPath, rand.Intn(9000)+1000,
	)

	req, err := http.NewRequestWithContext(ctx, "POST", rpcURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return p.urlProbe(ctx, owner, repo, result)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", p.client.userAgent)

	resp, err := p.client.client.Do(req)
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
	// 去掉 Google XSSI 防护前缀 )]}'\n
	bodyStr, _ = strings.CutPrefix(bodyStr, ")]}'\n")

	if !strings.Contains(bodyStr, "wrb.fr") || !strings.Contains(bodyStr, "VSX6ub") {
		return p.urlProbe(ctx, owner, repo, result)
	}

	// 信号分析
	signals := []string{"rpc_ok"}

	// 1. 负向信号：不存在索引的典型特征 [null,[[null,
	if strings.Contains(bodyStr, "[null,[[null,") {
		result.Status = StatusNotIndexed
		result.Confidence = "high"
		result.MatchedSignals = []string{"marker_not_indexed_null"}
		return result
	}

	// 2. 正向信号 A：包含 "owner/repo" 标识
	if strings.Contains(bodyStr, fmt.Sprintf("\"%s\"", fullName)) {
		signals = append(signals, "marker_repo_id_matched")
	}

	// 3. 正向信号 B：包含 Overview 关键字
	if strings.Contains(bodyStr, "Overview") {
		signals = append(signals, "marker_overview_found")
	}

	// 4. 正向信号 C：长度特征 (索引后的数据通常很大，非索引通常 < 300 字节)
	if len(bodyStr) > 5000 {
		signals = append(signals, "marker_large_payload")
	}

	result.MatchedSignals = signals

	// 判定逻辑
	if len(signals) >= 3 {
		result.Status = StatusIndexed
		result.Confidence = "high"
	} else if len(signals) >= 2 {
		result.Status = StatusProbablyIndexed
		result.Confidence = "medium"
	} else {
		// 虽然 RPC 通了，但没匹配到核心标识
		result.Status = StatusNotIndexed
		result.Confidence = "high"
	}

	return result
}
