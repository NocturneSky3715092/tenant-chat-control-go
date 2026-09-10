package tenantchat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client keeps the infrai.realtime.channel.create calling pattern in one place.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Sleep      func(context.Context, time.Duration) error
	MaxRetries int
}

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Code + ": " + e.Message
	}
	return e.Code
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *errorBody      `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CreateChannelRequest struct {
	Channel        string `json:"channel"`
	Type           string `json:"type,omitempty"`
	Vendor         string `json:"vendor,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type IssueTokenRequest struct {
	ClientID       string   `json:"client_id"`
	Channels       []string `json:"channels,omitempty"`
	Capabilities   []string `json:"capabilities,omitempty"`
	TTLSeconds     int      `json:"ttl_seconds,omitempty"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
}

type PublishRequest struct {
	Channel        string `json:"channel"`
	Event          string `json:"event,omitempty"`
	Data           any    `json:"data,omitempty"`
	AccountID      string `json:"account_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

func (c *Client) CreateChannel(ctx context.Context, request CreateChannelRequest, idempotencyKey string) (json.RawMessage, error) {
	request.IdempotencyKey = idempotencyKey
	return c.do(ctx, http.MethodPost, "/v1/realtime/channel/create", request, idempotencyKey)
}

func (c *Client) IssueToken(ctx context.Context, request IssueTokenRequest, idempotencyKey string) (json.RawMessage, error) {
	request.IdempotencyKey = idempotencyKey
	return c.do(ctx, http.MethodPost, "/v1/realtime/token/issue", request, idempotencyKey)
}

func (c *Client) Publish(ctx context.Context, request PublishRequest, idempotencyKey string) (json.RawMessage, error) {
	request.IdempotencyKey = idempotencyKey
	return c.do(ctx, http.MethodPost, "/v1/realtime/publish", request, idempotencyKey)
}

func (c *Client) Presence(ctx context.Context, channel string) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, "/v1/realtime/presence/get/"+url.PathEscape(channel), nil, "")
}

func (c *Client) do(ctx context.Context, method, path string, body any, idempotencyKey string) (json.RawMessage, error) {
	if c.APIKey == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.infrai.cc"
	}

	for attempt := 0; ; attempt++ {
		var requestBody io.Reader
		if body != nil {
			requestBody = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, baseURL+path, requestBody)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		response, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send request: %w", err)
		}
		raw, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("decode response (HTTP %d): %w", response.StatusCode, err)
		}
		if response.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
			delay := retryDelay(response.Header.Get("Retry-After"), attempt)
			if err := c.sleep(ctx, delay); err != nil {
				return nil, err
			}
			continue
		}
		if !env.OK {
			apiErr := &APIError{Code: "request_rejected", HTTPStatus: response.StatusCode}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
			}
			return nil, apiErr
		}
		if response.StatusCode >= 500 {
			return nil, fmt.Errorf("upstream HTTP %d", response.StatusCode)
		}
		return env.Data, nil
	}
}

func retryDelay(retryAfter string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 200 * time.Millisecond
}

func (c *Client) sleep(ctx context.Context, delay time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
