package tenantchat

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeRealtime struct {
	tokenCalls int
	published  []PublishRequest
}

func (f *fakeRealtime) CreateChannel(context.Context, CreateChannelRequest, string) (json.RawMessage, error) {
	return json.RawMessage(`{"channel":"tenant-acme"}`), nil
}

func (f *fakeRealtime) IssueToken(context.Context, IssueTokenRequest, string) (json.RawMessage, error) {
	f.tokenCalls++
	return json.RawMessage(`{"token":"client-token"}`), nil
}

func (f *fakeRealtime) Publish(_ context.Context, request PublishRequest, _ string) (json.RawMessage, error) {
	f.published = append(f.published, request)
	return json.RawMessage(`{"published":true}`), nil
}

func (f *fakeRealtime) Presence(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`{"members":[]}`), nil
}

func TestTenantAccountLifecycleControlsTokens(t *testing.T) {
	tests := []struct {
		name      string
		state     AccountState
		wantError error
		wantCalls int
	}{
		{name: "active account receives token", state: AccountActive, wantCalls: 1},
		{name: "suspended account is rejected locally", state: AccountSuspended, wantError: ErrAccountSuspended},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := &fakeRealtime{}
			service := NewTenantService(api)
			if _, err := service.Onboard(context.Background(), "acme"); err != nil {
				t.Fatal(err)
			}
			if test.state == AccountSuspended {
				if _, err := service.SetState(context.Background(), "acme", test.state); err != nil {
					t.Fatal(err)
				}
			}
			_, err := service.Token(context.Background(), "acme", "user-7")
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Token() error = %v, want %v", err, test.wantError)
			}
			if api.tokenCalls != test.wantCalls {
				t.Fatalf("token calls = %d, want %d", api.tokenCalls, test.wantCalls)
			}
		})
	}
}
