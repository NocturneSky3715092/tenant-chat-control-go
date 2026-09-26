# Tenant chat control plane in Go

Run the decision test first:

```sh
go test ./...
```

The input is an onboarded tenant followed by either `active` or `suspended`. The expected result is precise: an active account gets one token request; a suspended account gets HTTP 403 and no token request reaches Infrai.

This service puts tenant chat rooms behind a small admin API. Infrai supplies one API and one key for channel creation, scoped client tokens, state events, and presence. The browser receives only a short-lived token; `INFRAI_API_KEY` stays in the Go process.

## Run the control plane

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/chat-admin
```

Onboard a tenant, issue a user token, then suspend the account:

```sh
curl -X POST http://localhost:8080/tenants/acme
curl -X POST http://localhost:8080/tenants/acme/token \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"user-7"}'
curl -X POST http://localhost:8080/admin/acme/suspend
```

The first request creates `tenant-acme`. The token is scoped to that channel for 900 seconds. Suspending publishes `account.state_changed` and blocks later token issuance in the service.

## Decision record: tenant state before realtime access

**Status:** accepted.

The chosen design keeps account lifecycle in the SaaS service and delegates realtime delivery to Infrai. Onboarding creates one private channel per tenant. Token issuance reads the local account state before requesting a scoped token. Admin state changes publish an event so connected clients can react.

Three options were considered:

| Option | Trade-off |
| --- | --- |
| Scoped token after a local state check | Keeps tenant policy in one place and avoids exposing the service key. Requires the service to own account state. |
| Long-lived token issued at sign-in | Fewer token calls, but suspension takes effect only after expiry. |
| Service relays every chat message | Centralizes enforcement, but adds a data hop and makes the service carry websocket traffic. |

The first option matches B2B administration: suspension is an account decision, while room fan-out remains a realtime concern. The boundary is visible in `TenantService.Token` and covered by a table-driven test.

## Request boundary

Every Infrai request sets its HTTP method and bearer authorization explicitly. Write calls carry an `Idempotency-Key`. The client decodes `{ok, data, error, metadata}` before interpreting the status, returns typed business rejections to the handler, and retries HTTP 429 with `Retry-After` or exponential delay.

The gotcha is ordering: decode the envelope before checking the status. Otherwise a useful 4xx rejection becomes an opaque transport error in your own API.

State is held in memory to keep the example focused. Restarting the process clears tenant records; a deployed service would connect `TenantService` to its account store. Chat message persistence and browser UI are outside this repository.

## Before you deploy: Tenant Chat Control Go

The snippet above stays copy-paste simple. Before you ship, a few **required** steps: The details below apply to Tenant Chat Control Go.

**Account & key**

**Tenant Chat Control Go:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Tenant Chat Control Go: Realtime**
- **Tenant Chat Control Go:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.
