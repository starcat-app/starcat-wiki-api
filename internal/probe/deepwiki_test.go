package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeepWikiProbe(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus Status
	}{
		{
			name:       "Indexed High",
			status:     200,
			body:       "<html>last indexed on some date. overview for github.com/test/repo </html>",
			wantStatus: StatusIndexed,
		},
		{
			name:       "Probably Indexed",
			status:     200,
			body:       "<html>overview of the repo.</html>",
			wantStatus: StatusProbablyIndexed,
		},
		{
			name:       "Not Indexed (404)",
			status:     404,
			body:       "",
			wantStatus: StatusNotIndexed,
		},
		{
			name:       "Rate Limited (429)",
			status:     429,
			body:       "",
			wantStatus: StatusRateLimited,
		},
		{
			name:       "Rate Limited (403)",
			status:     403,
			body:       "",
			wantStatus: StatusRateLimited,
		},
		{
			name:       "Error (500)",
			status:     500,
			body:       "",
			wantStatus: StatusError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := &BaseRequest{client: server.Client()}
			probe := NewDeepWikiProbe(client)

			// Override the hardcoded URL generation just for test or mock transport.
			// The easiest way without changing signature is a custom transport.
			server.Client().Transport = &mockTransport{url: server.URL, body: []byte(tt.body), status: tt.status}

			res := probe.Probe(context.Background(), "test", "repo")
			if res.Status != tt.wantStatus {
				t.Errorf("got %v, want %v", res.Status, tt.wantStatus)
			}
		})
	}
}

type mockTransport struct {
	url    string
	body   []byte
	status int
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp := httptest.NewRecorder()
	resp.WriteHeader(m.status)
	resp.Write(m.body)
	return resp.Result(), nil
}
