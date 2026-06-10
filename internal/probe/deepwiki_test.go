package probe

import (
	"context"
	"testing"
)

// TestDeepWikiProbe 使用真实 DeepWiki API 探测指定 GitHub 仓库是否已被索引。
// 不依赖 mock / httptest，直接走网络请求。
func TestDeepWikiProbe(t *testing.T) {
	client := NewBaseRequest("")
	probe := NewDeepWikiProbe(client)

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
			res := probe.Probe(context.Background(), tt.owner, tt.repo)

			if res.Status != tt.wantStatus {
				t.Errorf(
					"Probe(%s/%s): status = %v, want %v (confidence=%s, error=%s, httpStatus=%v)",
					tt.owner, tt.repo,
					res.Status, tt.wantStatus,
					res.Confidence, res.Error,
					res.HTTPStatus,
				)
				return
			}

			t.Logf(
				"✓ %s/%s → status=%s confidence=%s signals=%v url=%s",
				tt.owner, tt.repo,
				res.Status, res.Confidence,
				res.MatchedSignals, res.URL,
			)
		})
	}
}
