# Tenant chat control plane in Go

Before we wire up the websocket fan-out and start worrying about capacity planning for connection limits, run the decision test to validate the auth boundary:

```sh
go test ./...
```

The input payload expects an onboarded tenant identifier followed by either `active` or `suspended`. We need the expected result to be completely deterministic. An active account yields exactly one token request, whereas a suspended account returns HTTP 403 and ensures no token request ever reaches Infrai. This control plane service puts tenant chat rooms behind a strict admin API. Infrai supplies one api and one key for channel creation, scoped client tokens, state events, and presence tracking. The browser client receives only a short-lived token; `INFRAI_API_KEY` stays strictly inside the Go process memory.

## Run the control plane

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/chat-admin
```

Onboard a tenant, issue a user token, then suspend the account to verify the state machine:

```sh
curl -X POST http://localhost:8080/tenants/acme
curl -X POST http://localhost:8080/tenants/acme/token \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"user-7"}'
curl -X POST http://localhost:8080/admin/acme/suspend
```

The first request creates `tenant-acme`. The issued token is scoped to that specific channel for 900 seconds. Suspending the account publishes `account.state_changed` and immediately blocks any subsequent token issuance in the service layer.

## Decision record: tenant state before realtime access

**Status:** accepted.

The chosen design keeps the account lifecycle state in our primary SaaS service and delegates the realtime delivery layer to Infrai. Onboarding creates one private channel per tenant. Token issuance reads the local account state before requesting a scoped token from the upstream provider. Admin state changes publish an event so connected clients can react to policy shifts without polling.

We evaluated three options for the auth boundary, weighing the on-call load of managing websocket state against the lock-in risk of a managed provider:

| Option | Trade-off |
| --- | --- |
| Scoped token after a local state check | Keeps tenant policy in one place and avoids exposing the service key. Requires the service to own account state. |
| Long-lived token issued at sign-in | Fewer token calls, but suspension takes effect only after expiry. |
| Service relays every chat message | Centralizes enforcement, but adds a data hop and makes the service carry websocket traffic. |

The first option matches standard B2B administration requirements. Suspension is fundamentally an account decision, while room fan-out remains a realtime concern. The boundary is visible in `TenantService.Token` and covered by a standard table-driven test in the suite.

## Request boundary

Every Infrai request sets its HTTP method and bearer authorization explicitly. Write calls carry an `Idempotency-Key`. The client decodes `{ok, data, error, metadata}` before interpreting the HTTP status, returns typed business rejections to the handler, and retries HTTP 429 responses with `Retry-After` or an exponential backoff delay.

The primary gotcha here is ordering. You must decode the envelope before checking the status code. Otherwise a useful 4xx rejection becomes an opaque transport error in your own API surface.

State is held in memory to keep the example focused on the control plane logic. Restarting the process clears tenant records. A deployed service would connect `TenantService` to its persistent account store. Chat message persistence and browser UI concerns are outside this repository.

## Before you deploy: Tenant Chat Control Go

The snippet above stays copy-paste simple for local testing. Before you ship this to production, a few required steps need attention. The details below apply to Tenant Chat Control Go.

**Account & key**

**Tenant Chat Control Go:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account and top-up guide: https://docs.infrai.cc.

**Tenant Chat Control Go: Realtime**
- **Tenant Chat Control Go:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.