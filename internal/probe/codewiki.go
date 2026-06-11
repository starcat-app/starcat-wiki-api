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

// urlProbe 简单 HTTP GET 探测（备用方案）。
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
		return result
	}

	if resp.StatusCode == http.StatusOK {
		result.Status = StatusError
		result.Error = "url_probe_indeterminate"
		return result
	}

	result.Status = StatusError
	result.Error = fmt.Sprintf("http_%d", resp.StatusCode)
	return result
}

// rpcProbe 通过 Google batchexecute RPC 探测（精确）。
//
// 原理：调用 codewiki.google 的 batchexecute 接口传入 GitHub URL，
// 分析响应中的信号判断是否已被 Google Code Wiki 索引。
// 无需登录，在无痕模式下也可工作。
func (p *CodeWikiProbe) rpcProbe(ctx context.Context, owner, repo string, result ProbeResult) ProbeResult {
	result.ProbeMethod = "batchexecute_fetch"

	ghURL := fmt.Sprintf("https://github.com/%s/%s", owner, repo)
	fullName := fmt.Sprintf("%s/%s", owner, repo)

	// source-path 必须 URL 编码
	encodedPath := url.QueryEscape(fmt.Sprintf("/github.com/%s/%s", owner, repo))

	// f.req 载荷：batchexecute RPC 格式，3 层括号，传入完整 GitHub URL
	payload := fmt.Sprintf(`[[["VSX6ub","[\"%s\"]",null,"generic"]]]`, ghURL)
	formData := url.Values{}
	formData.Set("f.req", payload)

	rpcURL := fmt.Sprintf(
		"https://codewiki.google/_/BoqAngularSdlcAgentsUi/data/batchexecute?rpcids=VSX6ub&source-path=%s&rt=c&_reqid=%d",
		encodedPath, rand.Intn(9000)+1000,
	)

	req, err := http.NewRequestWithContext(ctx, "POST", rpcURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return p.fallbackError(result, "new_request_error", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", p.client.userAgent)

	resp, err := p.client.client.Do(req)
	if err != nil {
		return p.fallbackError(result, "rpc_do_error", err)
	}
	defer resp.Body.Close()

	result.HTTPStatus = &resp.StatusCode

	if resp.StatusCode != http.StatusOK {
		result.Status = StatusError
		result.Error = fmt.Sprintf("rpc_http_%d", resp.StatusCode)
		return result
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return p.fallbackError(result, "rpc_read_body_error", err)
	}

	bodyStr := string(body)
	// 去掉 Google XSSI 防护前缀 )]}'\n
	bodyStr, _ = strings.CutPrefix(bodyStr, ")]}'\n")

	if !strings.Contains(bodyStr, "wrb.fr") || !strings.Contains(bodyStr, "VSX6ub") {
		result.Status = StatusError
		result.Error = "rpc_response_format_unexpected"
		return result
	}

	// 信号分析
	signals := []string{"rpc_ok"}

	// 负向信号：未索引的典型特征 [null,[[null,
	if strings.Contains(bodyStr, "[null,[[null,") {
		result.Status = StatusNotIndexed
		result.MatchedSignals = []string{"marker_not_indexed_null"}
		return result
	}

	// 正向信号 A：包含 "owner/repo" 标识
	if strings.Contains(bodyStr, fmt.Sprintf("\"%s\"", fullName)) {
		signals = append(signals, "marker_repo_id_matched")
	}

	// 正向信号 B：包含 Overview 关键字
	if strings.Contains(bodyStr, "Overview") {
		signals = append(signals, "marker_overview_found")
	}

	// 正向信号 C：长度特征（索引后的数据通常 > 5KB）
	if len(bodyStr) > 5000 {
		signals = append(signals, "marker_large_payload")
	}

	result.MatchedSignals = signals

	// 判定：≥2 个正向信号即可判定为 indexed
	// （v2 简化：probably_indexed → indexed）
	if len(signals) >= 2 {
		result.Status = StatusIndexed
	} else {
		result.Status = StatusNotIndexed
	}

	return result
}

// fallbackError 构造 RPC 降级错误结果。
func (p *CodeWikiProbe) fallbackError(result ProbeResult, prefix string, err error) ProbeResult {
	result.Status = StatusError
	result.Error = fmt.Sprintf("%s: %v", prefix, err)
	return result
}
