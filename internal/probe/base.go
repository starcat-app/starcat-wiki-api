package probe

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"time"
)

type BaseRequest struct {
	client    *http.Client
	userAgent string
}

func NewBaseRequest(ua string) *BaseRequest {
	if ua == "" {
		ua = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
	}
	return &BaseRequest{
		client:    &http.Client{Timeout: 30 * time.Second},
		userAgent: ua,
	}
}

func (b *BaseRequest) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	b.setHeaders(req)
	return b.client.Do(req)
}

func (b *BaseRequest) Post(ctx context.Context, url string, body interface{}) (*http.Response, error) {
	// Simple wrapper for now, can be extended if needed
	return nil, fmt.Errorf("not implemented")
}

func (b *BaseRequest) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", b.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,zh-CN;q=0.8")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
}
func RandomDelay(min, max int) {
	if max <= min {
		return
	}
	d := rand.Intn(max-min) + min
	time.Sleep(time.Duration(d) * time.Millisecond)
}
