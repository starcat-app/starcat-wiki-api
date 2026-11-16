package probe

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type zreadRoundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip zreadRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

// TestZreadProbe 使用固定响应，避免上游仓库索引状态变化影响单测结果。
func TestZreadProbe(testContext *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantStatus Status
		wantError  string
	}{
		{
			name:       "已索引",
			statusCode: http.StatusOK,
			body:       "{\"code\":0,\"data\":{\"status\":\"success\"}}",
			wantStatus: StatusIndexed,
		},
		{
			name:       "未索引",
			statusCode: http.StatusOK,
			body:       "{\"code\":0,\"data\":{\"status\":\"failed\"}}",
			wantStatus: StatusNotIndexed,
		},
		{
			name:       "上游限流",
			statusCode: http.StatusTooManyRequests,
			wantStatus: StatusError,
			wantError:  "http_429",
		},
		{
			name:       "无效响应",
			statusCode: http.StatusOK,
			body:       "invalid JSON",
			wantStatus: StatusError,
			wantError:  "json_decode_error:",
		},
	}

	for _, testCase := range tests {
		testContext.Run(testCase.name, func(caseContext *testing.T) {
			client := NewBaseRequest("")
			client.client.Transport = zreadRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.String() != "https://zread.ai/api/v1/repo/github/microsoft/vscode" {
					caseContext.Errorf("unexpected request URL: %s", request.URL)
				}
				return &http.Response{
					StatusCode: testCase.statusCode,
					Body:       io.NopCloser(strings.NewReader(testCase.body)),
					Header:     make(http.Header),
				}, nil
			})
			result := NewZreadProbe(client).Probe(context.Background(), "microsoft", "vscode")
			if result.Status != testCase.wantStatus {
				caseContext.Errorf("status = %s, want %s", result.Status, testCase.wantStatus)
			}
			if testCase.wantError != "" && !strings.HasPrefix(result.Error, testCase.wantError) {
				caseContext.Errorf("error = %q, want prefix %q", result.Error, testCase.wantError)
			}
			if result.URL != "https://zread.ai/microsoft/vscode" {
				caseContext.Errorf("URL = %q", result.URL)
			}
		})
	}
}
