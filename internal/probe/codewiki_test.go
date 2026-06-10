package probe

import (
	"context"
	"net/http"
	"testing"
)

func TestCodeWikiProbe(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus Status
		enableRPC  bool
	}{
		{
			name:       "URL Probe - 404",
			status:     404,
			body:       "",
			wantStatus: StatusNotIndexed,
			enableRPC:  false,
		},
		{
			name:       "URL Probe - 200",
			status:     200,
			body:       "some content",
			wantStatus: StatusUnknown, // url probe 200 => unknown
			enableRPC:  false,
		},
		{
			name:       "RPC Probe - Success",
			status:     200,
			body:       "wrb.fr VSX6ub canonicalUrl https://github.com/test/repo sections",
			wantStatus: StatusIndexed,
			enableRPC:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &BaseRequest{client: &http.Client{
				Transport: &mockTransport{status: tt.status, body: []byte(tt.body)},
			}}
			
			// 对于 RPC，http.DefaultClient 也需要被 mock，因为实现里写死了 http.DefaultClient
			oldClient := http.DefaultClient
			http.DefaultClient = client.client
			defer func() { http.DefaultClient = oldClient }()

			probe := NewCodeWikiProbe(client, tt.enableRPC)

			res := probe.Probe(context.Background(), "test", "repo")
			if res.Status != tt.wantStatus {
				t.Errorf("got %v, want %v", res.Status, tt.wantStatus)
			}
		})
	}
}
