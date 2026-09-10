package tenantchat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

type RealtimeAPI interface {
	CreateChannel(context.Context, CreateChannelRequest, string) (json.RawMessage, error)
	IssueToken(context.Context, IssueTokenRequest, string) (json.RawMessage, error)
	Publish(context.Context, PublishRequest, string) (json.RawMessage, error)
	Presence(context.Context, string) (json.RawMessage, error)
}

type AccountState string

const (
	AccountActive    AccountState = "active"
	AccountSuspended AccountState = "suspended"
)

var ErrAccountSuspended = errors.New("account is suspended")

type TenantAccount struct {
	TenantID string       `json:"tenant_id"`
	Channel  string       `json:"channel"`
	State    AccountState `json:"state"`
}

type TenantService struct {
	realtime RealtimeAPI
	mu       sync.RWMutex
	accounts map[string]TenantAccount
}

func NewTenantService(realtime RealtimeAPI) *TenantService {
	return &TenantService{realtime: realtime, accounts: make(map[string]TenantAccount)}
}

func (s *TenantService) Onboard(ctx context.Context, tenantID string) (TenantAccount, error) {
	channel := "tenant-" + tenantID
	if _, err := s.realtime.CreateChannel(ctx, CreateChannelRequest{Channel: channel, Type: "private"}, "onboard-"+tenantID); err != nil {
		return TenantAccount{}, err
	}
	account := TenantAccount{TenantID: tenantID, Channel: channel, State: AccountActive}
	s.mu.Lock()
	s.accounts[tenantID] = account
	s.mu.Unlock()
	return account, nil
}

func (s *TenantService) SetState(ctx context.Context, tenantID string, state AccountState) (TenantAccount, error) {
	s.mu.Lock()
	account, ok := s.accounts[tenantID]
	if !ok {
		s.mu.Unlock()
		return TenantAccount{}, errors.New("tenant not found")
	}
	account.State = state
	s.accounts[tenantID] = account
	s.mu.Unlock()

	_, err := s.realtime.Publish(ctx, PublishRequest{
		Channel:   account.Channel,
		Event:     "account.state_changed",
		Data:      map[string]string{"state": string(state)},
		AccountID: tenantID,
	}, fmt.Sprintf("state-%s-%s", tenantID, state))
	return account, err
}

func (s *TenantService) Token(ctx context.Context, tenantID, userID string) (json.RawMessage, error) {
	s.mu.RLock()
	account, ok := s.accounts[tenantID]
	s.mu.RUnlock()
	if !ok {
		return nil, errors.New("tenant not found")
	}
	if account.State != AccountActive {
		return nil, ErrAccountSuspended
	}
	return s.realtime.IssueToken(ctx, IssueTokenRequest{
		ClientID:     userID,
		Channels:     []string{account.Channel},
		Capabilities: []string{"subscribe", "publish", "presence"},
		TTLSeconds:   900,
	}, "token-"+tenantID+"-"+userID)
}

func (s *TenantService) Presence(ctx context.Context, tenantID string) (json.RawMessage, error) {
	s.mu.RLock()
	account, ok := s.accounts[tenantID]
	s.mu.RUnlock()
	if !ok {
		return nil, errors.New("tenant not found")
	}
	return s.realtime.Presence(ctx, account.Channel)
}
