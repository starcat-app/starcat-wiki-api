package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerAuth(t *testing.T) {
	auth := NewBearerAuth([]string{"test-key-1", "test-key-2"})
	
	tests := []struct{
		name   string
		header string
		status int
	}{
		{"Valid Key 1", "Bearer test-key-1", http.StatusOK},
		{"Valid Key 2", "Bearer test-key-2", http.StatusOK},
		{"Missing Header", "", http.StatusUnauthorized},
		{"Wrong Format", "test-key-1", http.StatusUnauthorized},
		{"Invalid Key", "Bearer invalid", http.StatusUnauthorized},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := auth.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			
			req := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			
			if rr.Code != tt.status {
				t.Errorf("expected status %d, got %d", tt.status, rr.Code)
			}
		})
	}
}
