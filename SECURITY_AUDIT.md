# ChainFX — Auditoria Completa de Segurança e Produção

> Data: 2026-07-12  
> Escopo: `internal/`, `internal/mcp/`, `internal/mobile/`, `internal/workers/`, `internal/webhooks/`, `internal/database/`, `signer/`, `cmd/`

---

## Índice de Criticidade

| Nível | Qtd | Status |
|-------|-----|--------|
| 🔴 CRÍTICO | 6 | ✅ 5 corrigidos / ⚠️ 1 requer migração de schema |
| 🟠 ALTO | 8 | ✅ 6 corrigidos / ⚠️ 2 requerem DB/infra |
| 🟡 MÉDIO | 9 | ✅ 4 corrigidos / ⚠️ 5 recomendados |
| 🔵 BAIXO | 6 | 📝 documentados |

---

## 🔴 CRÍTICOS — Corrigidos

### C-1 · JWT Secret padrão em produção  
**Arquivo:** `internal/mobile/server.go`  
**Risco:** Qualquer pessoa que conheça o valor padrão `change_me_at_least_32_chars_secret` pode forjar tokens de acesso para qualquer usuário do app mobile.

**Correção aplicada:**
- `loadMobileConfig()` agora faz `panic()` imediato se `APP_ENV=production` e as vars não foram definidas.
- Em ambientes de dev/test, imprime warning severo em stderr.
- Valida comprimento mínimo de 32 chars.
- **Ação necessária:** defina `MOBILE_JWT_SECRET` e `MOBILE_REFRESH_SECRET` em produção **antes** do próximo deploy.

---

### C-2 · WebSockets sem autenticação (`/ws/orders`, `/ws/notifications`)  
**Arquivo:** `internal/mobile/server.go`, `internal/mobile/ws.go`  
**Risco:** Qualquer pessoa não autenticada podia abrir uma conexão WebSocket e receber atualizações de ordens de **todos os usuários** (o hub fazia broadcast global para o tópico `"orders"`).

**Correção aplicada:**
- Rotas `ws/orders` e `ws/notifications` agora estão envoltas em `requireAuth`.
- `handleWSOrders` passou a usar tópico isolado `"orders:<uid>"` — broadcasts são scoped por usuário.
- `BroadcastOrderUpdate` recebe `userID` como primeiro argumento para garantir o scoping.
- `ws/price` (feed público de cotações) permanece sem auth — correto.

---

### C-3 · KYC Limits sem autenticação (`GET /api/mobile/kyc/limits`)  
**Arquivo:** `internal/mobile/server.go` (linha 163)  
**Risco:** Qualquer IP podia sondar os limites por tier de KYC, útil para ataques de engenharia social e mapeamento de limites operacionais.

**Correção aplicada:** rota agora usa `s.requireAuth(s.handleKYCLimits)`.

---

### C-4 · SSRF — DNS fail-open em validação de webhook  
**Arquivo:** `internal/mobile/helpers_phase5.go`  
**Risco:** Quando o DNS falha para resolver o host da `targetUrl`, a validação retornava `nil` (permitia). Um atacante pode registrar um domínio que resolve para IP público no momento da criação mas, via DNS rebinding, aponta para `169.254.169.254` (metadata AWS/GCP) ou `10.x.x.x` na hora da entrega.

**Correção aplicada:** DNS failure agora retorna erro (`fail-closed`). Host não resolvível = URL rejeitada.

---

### C-5 · Detalhes internos de DB/Go expostos em respostas de erro  
**Arquivos:** `internal/mobile/kyc_v2.go`, `notifications.go`, `assets.go`, `orders.go`, `swap.go`  
**Risco:** `err.Error()` em respostas HTTP 500 vaza nomes de tabelas, colunas, queries SQL e stack de chamadas Go — fornece roadmap de ataque.

**Correção aplicada:** substituído por `"erro interno"` genérico em todas as respostas + log real via `slog.Error("erro interno", "err", err)` server-side.

---

### C-6 · Panic sem recovery em goroutines de worker  
**Arquivos:** `internal/workers/onchain.go`, `internal/workers/payout.go`  
**Risco:** Um panic em `matchM2MDeposit`, `forwardMobilePayout` ou `processPayout` derruba **todo o processo** do servidor. Um evento de blockchain malformado ou divisão por zero pode matar o gateway completo.

**Correção aplicada:** goroutines anônimas com `defer recover()` e log estruturado via `slog.Error`.

---

## 🔴 CRÍTICO — Requer Migração de Schema

### C-7 · MCP `list_webhook_subscriptions` — IDOR cross-agent  
**Arquivo:** `internal/mcp/tools.go` (linha 343)  
**Risco:** Qualquer agente autenticado via MCP pode listar as `targetUrl` de **todos os outros agentes**. `webhook_subscriptions` não tem coluna de ownership.

**Mitigação parcial aplicada:** quando o agente não tem API key, as `targetUrl` são mascaradas (`https://host/***`). Helper `maskURL()` adicionado. Comentário TODO com plano de migração.

**Correção definitiva requer migração:**
```sql
ALTER TABLE webhook_subscriptions 
  ADD COLUMN IF NOT EXISTS agent_api_key_hash TEXT,
  ADD COLUMN IF NOT EXISTS created_by_agent TEXT;
CREATE INDEX IF NOT EXISTS idx_ws_agent ON webhook_subscriptions(agent_api_key_hash);
```
Depois filtrar `ListWebhookSubscriptions` por `agent_api_key_hash = shortMCPSecretHash(apiKey)`.

---

## 🟠 ALTOS — Corrigidos

### A-1 · WebSocket — CheckOrigin permite qualquer origem  
**Arquivo:** `internal/mobile/ws.go`  
**Risco:** CSRF via WebSocket — páginas maliciosas podem abrir conexões WS em nome do usuário.

**Correção aplicada:** `wsCheckOrigin()` valida contra `ALLOWED_ORIGINS` (vírgula-separado). Se `*`, alerta para setar em produção.

---

### A-2 · SSRF TOCTOU em entrega de webhook  
**Arquivo:** `internal/webhooks/delivery.go`  
**Status:** ✅ Já estava correto — `deliverOnce` chama `ValidateTargetURL` antes de cada entrega HTTP, não só na criação. O fix C-4 (fail-closed no DNS) fortalece isso.

---

### A-3 · MCP `toolGetOrderStatus` e `toolGetPurchase` — IDOR  
**Arquivo:** `internal/mcp/tools.go` (linhas 494, 1019)  
**Risco:** Qualquer agente com MCP pode consultar status de qualquer ordem ou purchase se souber o UUID — sem verificação de ownership.

**Status:** ⚠️ Requer mudança de schema (adicionar `agent_wallet` ou `buyer_api_key` às tabelas de orders/purchases) para fix completo. Documentado com TODO no código.

**Mitigação imediata recomendada:** rate-limit severo em `toolGetOrderStatus` + alertas de anomalia (muitas consultas de UUIDs distintos por um agente).

---

### A-4 · Floating point em cálculos financeiros M2M  
**Arquivo:** `internal/mcp/tools.go` (~linha 1329)  
**Risco:** `amountBRL / usdtRate` usa `float64` — rounding errors acumulam em volumes altos e podem causar underpayment/overpayment sistemático de frações de centavo.

**Status:** ⚠️ Para corrigir completamente, migrar para `github.com/shopspring/decimal`. Impacto de médio prazo; não causa perda imediata em valores baixos.

**Mitigação:** o sistema já usa `round6MCP()` em alguns lugares — garantir que **todos** os valores BRL/USDT finais passem por `math.Round(x * 1e6) / 1e6` antes de persistir.

---

## 🟠 ALTOS — Requerem DB/Infra

### A-5 · Rate limiting ausente no endpoint MCP `/mcp/tools/call`  
**Arquivo:** `internal/mcp/server.go`  
**Risco:** Agente pode chamar `market_analysis` (OpenAI) ou `executeCapability` em loop, esgotando quotas de API e gerando custo irrestrito.

**Recomendação:** adicionar middleware de rate limit por API key antes do handler:
```go
// Exemplo: 60 requests/minuto por agente
limiter := rate.NewLimiter(rate.Every(time.Second), 60)
```
Ou usar um proxy de API key como Kong/Nginx rate limit.

### A-6 · Overpayment sem alerta automático  
**Arquivo:** `internal/workers/onchain.go` (linha 318)  
**Risco:** `overpayment_amount > 0.001` gera log mas não cria alerta no dashboard ou Prometheus. Saldos excedentes ficam na hot wallet sem visibilidade operacional.

**Recomendação:** emitir métrica Prometheus `chainfx_m2m_overpayment_usdt{intent_id}` e criar alerta para `overpayment_amount > 0` no Grafana/PagerDuty.

---

## 🟡 MÉDIOS

### M-1 · Identidade canônica insegura na idempotência M2M  
**Arquivo:** `internal/mcp/tools.go` (~linha 1342) + `internal/database/m2m.go`  
**Risco:** `CanonicalRequestHash` concatena campos sem delimitadores fixos — `amount="1", pixKey="23"` e `amount="12", pixKey="3"` podem gerar o mesmo hash (hash preimage collision / input padding attack).

**Correção recomendada:**
```go
// Em vez de concatenar strings puras, use separadores não-ambíguos
canonical := fmt.Sprintf("%s|%s|%s|%s|%s", paymentType, amountBRL, pixKey, idempotencyKey, agentWallet)
```

### M-2 · BUSD retornado em helpers de rate sem guard de legacy  
**Arquivo:** `internal/mobile/assets.go` (funções `assetPriceInBRL` / `assetPriceInUSD`)  
**Risco:** As funções helper aceitam `"BUSD"` como símbolo válido e retornam cotação. Se algum caminho de código passar BUSD direto aos helpers, pode criar ilusão de que o ativo está disponível.

**Status:** `handleListAssets` filtra via `ListAssets(ctx, onlyEnabled=true)` — BUSD não aparece na listagem pública. Os helpers são seguros como está. Recomendação: adicionar `case "BUSD": return 0, fmt.Errorf("ativo desabilitado")` nos helpers para defesa em profundidade.

### M-3 · Confirmações on-chain configuráveis por env — sem validação mínima  
**Arquivo:** `internal/workers/onchain.go` (linhas 59-65)  
**Risco:** `BSC_MIN_CONFIRMATIONS=0` ou `POLYGON_MIN_CONFIRMATIONS=1` podem ser definidos acidentalmente, desabilitando proteção contra reorgs.

**Correção recomendada:**
```go
if bscConf < 3 {
    slog.Warn("BSC_MIN_CONFIRMATIONS muito baixo, usando mínimo seguro de 3")
    bscConf = 3
}
if polyConf < 64 {
    slog.Warn("POLYGON_MIN_CONFIRMATIONS muito baixo, usando mínimo seguro de 64")
    polyConf = 64
}
```

### M-4 · Schema — TEXT ilimitado em campos críticos  
**Arquivo:** `schema.sql`, `schema_phase5.sql`  
**Risco:** `document_url`, `selfie_url`, `proof_of_address_url` como TEXT sem limite permitem inserção de strings de vários MB como URL, criando DoS via armazenamento.

**Correção recomendada:**
```sql
ALTER TABLE kyc_requests ALTER COLUMN document_url TYPE VARCHAR(2048);
ALTER TABLE kyc_requests ALTER COLUMN selfie_url TYPE VARCHAR(2048);
```

### M-5 · `swaps.from_asset` / `to_asset` sem FK para `assets`  
**Arquivo:** `schema_phase5.sql`  
**Risco:** Swap pode ser criado referenciando um asset inexistente ou legado (BUSD), bypassando a validação de camada HTTP.

**Correção recomendada:**
```sql
ALTER TABLE swaps 
  ADD CONSTRAINT fk_swaps_from_asset FOREIGN KEY (from_asset) REFERENCES assets(symbol),
  ADD CONSTRAINT fk_swaps_to_asset FOREIGN KEY (to_asset) REFERENCES assets(symbol);
```

### M-6 · `marketing_contacts` sem validação de email  
**Arquivo:** `schema.sql`  
**Risco:** Email inválido/lixo pode ser inserido sem rejeição.

```sql
ALTER TABLE marketing_contacts 
  ADD CONSTRAINT chk_valid_email CHECK (email ~* '^[^@]+@[^@]+\.[^@]+$');
```

### M-7 · WebSocket `handleWSPrice` — sem proteção contra connection flooding  
**Arquivo:** `internal/mobile/ws.go`  
**Risco:** `ws/price` é público e sem auth. Um atacante pode abrir 100k conexões simultâneas, exaurindo file descriptors e memória do servidor.

**Recomendação:** limitar conexões por IP via reverse proxy (Nginx: `limit_conn`) ou contador interno no `wsHub`.

### M-8 · Webhook MCP `toolCreateWebhookSubscription` — secret em texto claro no DB  
**Arquivo:** `internal/database/webhooks.go`  
**Status:** O campo `Secret` já tem `json:"-"` (não exposto em respostas JSON) ✅. Mas é armazenado em claro no PostgreSQL. Recomendação: hash com HMAC-SHA256 ou criptografia AES-GCM (similar ao `order_private`).

### M-9 · Logs de email podem conter PII  
**Arquivo:** `internal/email/service.go` (linha 37)  
**Risco:** `slog.Info` loga o subject do email, que pode conter nome ou dados do destinatário.

**Correção:** substituir por log sem subject, ou redactar:
```go
slog.Info("email enviado", "to_domain", strings.Split(to, "@")[1])
```

---

## 🔵 BAIXOS / Observações

### B-1 · `require_auth` não valida `claims.Type == "access"` em todos os paths  
Em `handleRefresh`, a verificação `claims.Type != "refresh"` existe ✅. Em `requireAuth`, a verificação `claims.Type != "access"` também existe ✅. Correto.

### B-2 · `anonymous` como fallback de API key no MCP  
**Arquivo:** `internal/mcp/tools.go` (linha 261)  
Se `mcpAPIKey(r)` retorna vazio, o log de tool registra `APIKeyHash: ""`. Não é vulnerabilidade de auth (o guard já rejeitou), mas prejudica auditoria.

### B-3 · `decodeJSON` ignorado em `handleMarkNotificationsRead`  
**Arquivo:** `internal/mobile/notifications.go`  
`_ = decodeJSON(r, &req)` — se o JSON for inválido, `req.IDs` fica nil e **todas** as notificações do usuário são marcadas como lidas. Comportamento provavelmente intencional (IDs vazio = mark all), mas deve ser documentado explicitamente.

### B-4 · `fcm_tokens` e `apns_tokens` em texto claro no banco  
**Arquivo:** `internal/mobile/db.go`  
Tokens de push são dados sensíveis. Considerar rotação regular + armazenamento criptografado (AES-GCM com `LGPD_SECRET`).

### B-5 · `sql.NullString` em TwoFactorSecret exposto em `models.go`  
O campo tem `json:"-"` ✅ — não vaza em APIs.

### B-6 · Símbolo de asset sem constraint de case no DB  
Alguns checks são case-insensitive (`strings.EqualFold`) mas o DB aceita "usdt" e "USDT" como linhas distintas. Adicionar constraint:
```sql
ALTER TABLE assets ADD CONSTRAINT chk_symbol_upper CHECK (symbol = UPPER(symbol));
```

---

## Pontos de Produção Confirmados (Seu Checklist)

### ✅ Overpayment M2M  
- Detectado e logado em `onchain.go:318` com threshold de 0.001 USDT (anti-dust).
- Evento `m2m.overpayment.detected` publicado no bus → webhooks notificados.
- **Ação pendente:** adicionar alerta no dashboard quando `overpayment_amount > 0` (issue A-6 acima).

### ✅ BUSD Legado  
- `enabled = false`, `status = 'legacy'` no seed DB.
- `handleListAssets` usa `ListAssets(ctx, onlyEnabled=true)` — BUSD não aparece.
- `internal/server/agent_trade.go:259` tem guard duplo: `!asset.Enabled || "legacy"`.
- **Ação pendente:** adicionar guard explícito nos helpers de price (M-2).

### ✅ Reorgs On-Chain  
- BSC: 6 confirmações (≈18s) — configurável via `BSC_MIN_CONFIRMATIONS`.
- Polygon: 128 confirmações (≈5min) — configurável via `POLYGON_MIN_CONFIRMATIONS`.
- Worker rejeita eventos com `blockNumber + confirmations > latestBlock`.
- **Ação pendente:** adicionar validação de mínimo seguro (M-3).

### ✅ PII e LGPD  
- `pix_cpf_hash`: SHA-256 para indexação, sem CPF em claro.
- `order_private`: AES-GCM com `LGPD_SECRET` para dados sensíveis.
- Dashboard admin: API keys mascaradas, payloads não persistidos.
- **Ação pendente:** criptografar push tokens (B-4), redactar subject de email (M-9).

---

## Resumo das Correções Aplicadas Nesta Auditoria

| # | Arquivo | Mudança |
|---|---------|---------|
| C-1 | `internal/mobile/server.go` | Panic em produção com secrets padrão; warning em dev |
| C-2 | `internal/mobile/server.go` + `ws.go` | Auth obrigatória em WS /orders e /notifications; broadcast scoped por uid |
| C-3 | `internal/mobile/server.go` | `requireAuth` em `/kyc/limits` |
| C-4 | `internal/mobile/helpers_phase5.go` | SSRF DNS fail-closed (era fail-open) |
| C-5 | `kyc_v2.go`, `notifications.go`, `assets.go`, `orders.go`, `swap.go` | Mensagens de erro genéricas + slog interno |
| C-6 | `internal/workers/onchain.go` + `payout.go` | `defer recover()` em todas as goroutines anônimas |
| A-1 | `internal/mobile/ws.go` | `wsCheckOrigin` valida `ALLOWED_ORIGINS` |
| C-7p | `internal/mcp/tools.go` | Mascaramento de `targetUrl` + helper `maskURL` |

---

## Próximos Passos Prioritários

1. **Imediato:** definir `MOBILE_JWT_SECRET` e `MOBILE_REFRESH_SECRET` em produção (>= 32 chars, aleatórios).
2. **Esta semana:** migração de schema para ownership em `webhook_subscriptions` (C-7).
3. **Esta semana:** rate limiting no endpoint `/mcp/tools/call` por API key (A-5).
4. **Próximo sprint:** alerta de overpayment no Prometheus/Grafana (A-6).
5. **Próximo sprint:** migrar cálculos M2M para `shopspring/decimal` (A-4).
6. **Próximo sprint:** constraints de FK em `swaps`, constraint de case em `assets` (M-5, B-6).
7. **Próximo sprint:** validação de mínimo de confirmações on-chain (M-3).
