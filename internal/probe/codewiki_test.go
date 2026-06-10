package probe

import (
	"context"
	"testing"
)

// TestCodeWikiProbe 使用真实 Google CodeWiki API 探测指定 GitHub 仓库是否已被索引。
// 启用 RPC 探测获取最准确的结果，不依赖 mock。
func TestCodeWikiProbe(t *testing.T) {
	client := NewBaseRequest("")

	tests := []struct {
		name       string
		owner      string
		repo       string
		wantStatus Status
	}{
		{
			name:       "已索引 — microsoft/vscode",
			owner:      "microsoft",
			repo:       "vscode",
			wantStatus: StatusIndexed,
		},
		{
			name:       "未索引 — dong4j/self-star-list",
			owner:      "dong4j",
			repo:       "self-star-list",
			wantStatus: StatusNotIndexed,
		},
	}

	for i, tt := range tests {
		if i > 0 {
			RandomDelay(300, 800)
		}

		t.Run(tt.name, func(t *testing.T) {
			// enableRPC=true：先走 RPC batchexecute，失败时 fallback 到 URL probe
			probe := NewCodeWikiProbe(client, true)

			res := probe.Probe(context.Background(), tt.owner, tt.repo)

			if res.Status != tt.wantStatus {
				t.Errorf(
					"Probe(%s/%s): status = %v, want %v (confidence=%s, error=%s, httpStatus=%v, signals=%v)",
					tt.owner, tt.repo,
					res.Status, tt.wantStatus,
					res.Confidence, res.Error,
					res.HTTPStatus, res.MatchedSignals,
				)
				return
			}

			t.Logf(
				"✓ %s/%s → status=%s confidence=%s method=%s signals=%v url=%s",
				tt.owner, tt.repo,
				res.Status, res.Confidence,
				res.ProbeMethod, res.MatchedSignals,
				res.URL,
			)
		})
	}
}
