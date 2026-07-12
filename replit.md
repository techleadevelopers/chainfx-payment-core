# ChainFX Payment Gateway

Backend Go para orquestração instantânea de settlement fiat ↔ USDT/cripto.

## Stack

- **Language**: Go 1.25.5
- **Database**: PostgreSQL
- **Blockchain**: BSC (BEP-20) via go-ethereum
- **Architecture**: HTTP API + background workers + mobile API layer

## How to run

```bash
# Requires DATABASE_URL env var (see .env.example in README)
go run ./cmd/api
```

The API starts on port **8080** (configurable via `PORT` env var).

## Project structure

| Path | Purpose |
|------|---------|
| `cmd/api` | Main API server entrypoint |
| `internal/server` | Web API routes (existing) |
| `internal/mobile` | `/api/mobile/*` routes for React Native app |
| `internal/workers` | Background workers (price, payout, buysend, onchain, sweep, KYC, swap, push notifications, webhooks) |
| `internal/database` | PostgreSQL queries |
| `internal/config` | Environment variable config |
| `internal/models` | Shared data models |
| `schema.sql` | Core DB schema |
| `schema_phase5.sql` | Phase 5 additions (must be applied to DB separately) |
| `signer/` | Isolated crypto signer service |
| `contracts/` | BSC smart contracts |

## Mobile API base path

All mobile endpoints live under `/api/mobile/` and are handled by `internal/mobile/Server`. They are additive — the existing web API at all other paths is untouched.

### Phase 5 mobile endpoints

| Feature | Endpoints |
|---------|-----------|
| Multi-Asset | `GET /api/mobile/assets`, `/assets/{symbol}`, `/assets/{symbol}/rate` |
| Multi-Country | `GET /api/mobile/countries`, `/countries/detect`, `/countries/{code}/rails` |
| KYC (async, non-blocking) | `POST /api/mobile/kyc/submit`, `GET /kyc/status`, `/kyc/history`, `/kyc/limits` |
| Swap (crypto→crypto) | `POST /api/mobile/swap/quote`, `/swap/execute`, `GET /swap/{id}`, `/swaps` |
| Webhooks (n8n/Zapier/Make) | `POST /api/mobile/webhooks/subscribe`, `GET /webhooks`, `DELETE /webhooks/{id}`, `PUT /webhooks/{id}/toggle` |

## Environment variables

See `README.md` for full list. Key vars:

```env
DATABASE_URL=postgres://...
MOBILE_JWT_SECRET=...
FCM_SERVER_KEY=...          # For push notifications
PIX_WEBHOOK_SECRET=...
SIGNER_URL=...
```

## Phase 5 DB migration

After setting `DATABASE_URL`, apply schemas in order:

```bash
psql $DATABASE_URL -f schema.sql
psql $DATABASE_URL -f schema_phase5.sql
psql $DATABASE_URL -f schema_agent_pricing.sql   # per-agent pricing policies
```

## MCP Capability Layer

ChainFX exposes a full **Model Context Protocol (MCP)** server at `/mcp/*`.

| Endpoint | Purpose |
|----------|---------|
| `POST /mcp/initialize` | Handshake / protocol version |
| `POST /mcp/tools/list` | All 30 tools (capabilities, payments, AI, webhooks) |
| `POST /mcp/tools/call` | Execute any tool |
| `POST /mcp/resources/list` | Resources (rates, capabilities, grants, policy, intents) |
| `POST /mcp/resources/read` | Read a resource by URI |
| `GET /mcp/test` | Connection health check |
| `GET /mcp/capabilities.json` | Public machine-readable capability registry |

### MCP Tools (production-ready)

**Capability Marketplace**: `listCapabilities`, `searchCapabilities`, `getCapability`, `getCapabilityContract`, `purchaseCapability`, `getPurchase`, `executeCapability`, `chooseRoute`, `getUsage`

**Agent Self-Service** *(new)*: `listAgentGrants`, `getAgentPolicy`, `dryRunCapability`, `listAgentPaymentIntents`

**M2M Payments**: `createPaymentIntent`, `getPaymentIntent` — PIX (real) with per-agent fee overrides

**Agent Rail**: `listAssets`, `quote`, `trade`, `settlementStatus`

**AI Analysis**: `market_analysis`, `trade_recommendation`, `price_prediction`, `detect_anomalies`, `summarize_transactions`

**Webhooks**: `list_webhook_events`, `create_webhook_subscription`, `list_webhook_subscriptions`, `trigger_test_webhook`

### Real Capability Adapters

| Capability | Provider | Status |
|------------|----------|--------|
| `semantic_memory` | Native PostgreSQL | ✅ Real |
| `llm_chat` | OpenAI (when `OPENAI_API_KEY` set) | ✅ Real |
| `document_ocr` | HTTP adapter (when `CAPABILITY_OCR_URL` set) | ✅ Real |
| `payments_fx` | M2M PIX/credit-card via Efí | ✅ Real |
| `aml_screening` | Structured mock (real provider TBD) | ⚠️ Demo |

### Per-Agent Pricing

Table `agent_pricing_policies` allows per-wallet fee overrides for M2M PIX, credit-card and capability take-rate. Null fields fall back to env-var globals.

### New Operational Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /app/intent/{id}` | M2M payment intent detail |
| `GET /app/risk` | M2M risk/settlement dashboard |
| `GET /agent/pricing/{wallet}` | Per-agent pricing policy |
| `GET /mcp/capabilities.json` | Public capability registry |

### Webhook Events

M2M lifecycle: `m2m.intent.created`, `m2m.deposit.received`, `m2m.settlement.done`, `m2m.settlement.failed`

Capability lifecycle: `capability.purchased`, `capability.executed`, `capability.granted`

## User preferences

- Keep mobile API isolated under `/api/mobile/` — never break existing web API routes
- Phase 5 is mobile-only; web API additions belong in `internal/server/`
- MCP tools must remain backward-compatible — never remove existing tool names
- Per-agent pricing overrides always fall back to env-var globals (never error)
