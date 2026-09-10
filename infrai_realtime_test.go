package tenantchat

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestClientDecodesEnvelopeBeforeStatusAndRetriesRateLimit(t *testing.T) {
	calls := 0
	client := &Client{
		BaseURL: "https://example.test",
		APIKey:  "test-key",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			status := http.StatusTooManyRequests
			body := `{"ok":false,"error":{"code":"busy","message":"retry later"}}`
			if calls == 2 {
				status = http.StatusOK
				body = `{"ok":true,"data":{"channel":"tenant-acme"},"metadata":{}}`
			}
			return &http.Response{
				StatusCode: status,
				Header:     http.Header{"Retry-After": []string{"1"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})},
		MaxRetries: 1,
		Sleep: func(_ context.Context, delay time.Duration) error {
			if delay != time.Second {
				t.Fatalf("delay = %s, want 1s", delay)
			}
			return nil
		},
	}

	data, err := client.CreateChannel(context.Background(), CreateChannelRequest{Channel: "tenant-acme"}, "onboard-acme")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !strings.Contains(string(data), "tenant-acme") {
		t.Fatalf("calls = %d, data = %s", calls, data)
	}
}
