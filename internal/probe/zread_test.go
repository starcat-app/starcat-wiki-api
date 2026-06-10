package probe

import (
	"context"
	"net/http"
	"testing"
)

func TestZreadProbe(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus Status
	}{
		{
			name:       "Indexed High",
			status:     200,
			body:       "<html>ask ai about source code overview for github.com/test/repo </html>",
			wantStatus: StatusIndexed,
		},
		{
			name:       "Not Indexed (404)",
			status:     404,
			body:       "",
			wantStatus: StatusNotIndexed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &BaseRequest{client: &http.Client{
				Transport: &mockTransport{status: tt.status, body: []byte(tt.body)},
			}}
			probe := NewZreadProbe(client)

			res := probe.Probe(context.Background(), "test", "repo")
			if res.Status != tt.wantStatus {
				t.Errorf("got %v, want %v", res.Status, tt.wantStatus)
			}
		})
	}
}
