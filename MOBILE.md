# ChainFX Mobile

## Produto

O módulo mobile expõe a API do app ChainFX para usuários finais comprarem, venderem e acompanharem stablecoins, carteiras, KYC, DCA, swaps, notificações e webhooks.

O produto mobile é a camada humana do gateway:

- compra de cripto com PIX/cartão;
- venda de USDT para PIX;
- carteira BSC do usuário;
- cotação e catálogo multi-asset;
- KYC e limites por nível;
- estratégias DCA;
- notificações e webhooks do usuário;
- status operacional via health check.

O mobile não substitui o MCP/agent rail. Ele atende usuário final em app React Native; o MCP atende agentes autônomos e integrações máquina-máquina.

## Fluxos Principais

### Autenticação

1. `POST /api/mobile/auth/register`
2. `POST /api/mobile/auth/login`
3. Cliente usa `Authorization: Bearer <accessToken>`
4. `POST /api/mobile/auth/refresh` renova sessão
5. `POST /api/mobile/auth/logout` revoga refresh token salvo

### Compra

1. App chama `POST /api/mobile/order/buy`.
2. Handler mobile encaminha para `/api/buy`.
3. Ordem é associada ao `user_id`.
4. Usuário acompanha em `/api/mobile/order/{id}` ou `/api/mobile/orders`.

### Venda / PIX

1. App chama `POST /api/mobile/order/sell` ou `POST /api/mobile/pix/generate`.
2. Handler encaminha para `/api/order`.
3. On-chain worker monitora depósito USDT.
4. Payout PIX é executado pelo worker principal.

### Carteira

`POST /api/mobile/wallet/generate` cria uma EOA e retorna a private key uma única vez para o cliente guardar. O backend salva apenas o endereço.

Importante: esse modelo é simples para MVP, mas coloca a geração de chave privada no backend. Para produção, preferir geração client-side ou MPC/custódia formal.

### KYC

Há dois fluxos:

- legado: `/api/mobile/user/kyc` e `/api/mobile/user/kyc/status`;
- v2 assíncrono: `/api/mobile/kyc/submit`, `/api/mobile/kyc/status`, `/api/mobile/kyc/history`, `/api/mobile/kyc/limits`.

O worker `KYCWorker` consome eventos `kyc.submitted` e aprova níveis 1 e 2 automaticamente; nível 3 fica em revisão.

## Rotas

### Públicas

- `GET /api/mobile/health`
- `GET /api/mobile/assets`
- `GET /api/mobile/assets/{symbol}`
- `GET /api/mobile/assets/{symbol}/rate`
- `GET /api/mobile/countries`
- `GET /api/mobile/countries/detect`
- `GET /api/mobile/countries/{code}`
- `GET /api/mobile/countries/{code}/rails`
- `GET /api/mobile/webhooks/events`
- `GET /api/mobile/ws/price`

### Autenticadas

- Auth: `POST /api/mobile/auth/logout`
- User: `GET|PUT /api/mobile/user/profile`
- KYC: `POST /api/mobile/user/kyc`, `GET /api/mobile/user/kyc/status`, `POST /api/mobile/kyc/submit`, `GET /api/mobile/kyc/status`, `GET /api/mobile/kyc/history`, `GET /api/mobile/kyc/limits`
- Wallet: `GET /api/mobile/wallet/balance`, `GET /api/mobile/wallet/tokens`, `GET /api/mobile/wallet/address`, `POST /api/mobile/wallet/generate`, `GET /api/mobile/wallet/history`
- Orders: `POST /api/mobile/order/buy`, `POST /api/mobile/order/sell`, `POST /api/mobile/order/swap`, `GET /api/mobile/order/{id}`, `GET /api/mobile/orders`, `POST /api/mobile/order/cancel`
- PIX: `POST /api/mobile/pix/generate`, `GET /api/mobile/pix/status/{id}`, `POST /api/mobile/pix/copy`
- DCA: `POST /api/mobile/dca/create`, `GET /api/mobile/dca/strategies`, `PUT /api/mobile/dca/{id}`, `DELETE /api/mobile/dca/{id}`, `GET /api/mobile/dca/{id}/status`
- Security: `POST /api/mobile/security/pin`, `POST /api/mobile/security/biometry`, `POST /api/mobile/security/2fa`, `GET /api/mobile/security/devices`, `DELETE /api/mobile/security/device`
- Contracts: `POST /api/mobile/contracts/payout`, `GET /api/mobile/contracts/vault`, `GET /api/mobile/contracts/delegate`, `POST /api/mobile/contracts/pause`, `POST /api/mobile/contracts/unpause`
- Notifications: `GET /api/mobile/notifications`, `PUT /api/mobile/notifications/read`, `DELETE /api/mobile/notifications/{id}`, `POST /api/mobile/notifications/token`
- Swap: `POST /api/mobile/swap/quote`, `POST /api/mobile/swap/execute`, `GET /api/mobile/swap/{id}`, `GET /api/mobile/swaps`
- User webhooks: `POST /api/mobile/webhooks/subscribe`, `GET /api/mobile/webhooks`, `DELETE /api/mobile/webhooks/{id}`, `PUT /api/mobile/webhooks/{id}/toggle`
- WebSocket: `/api/mobile/ws/orders`, `/api/mobile/ws/notifications`

### Provider/Webhook

- `POST /api/mobile/pix/confirm`

Essa rota é pública porque é pensada como webhook de provider. Ela deve permanecer protegida indiretamente pela validação/HMAC do handler interno `/api/pix/webhook`.

## Configuração

Variáveis principais:

- `MOBILE_JWT_SECRET`: segredo HS256 do access token, mínimo 32 chars.
- `MOBILE_REFRESH_SECRET`: segredo HS256 do refresh token, mínimo 32 chars recomendado.
- `MOBILE_JWT_EXPIRES_MIN`: TTL do access token, default 15.
- `MOBILE_REFRESH_EXPIRES_DAYS`: TTL do refresh token, default 7.
- `FCM_SERVER_KEY`: push notification.
- `ALLOWED_ORIGINS`: origens permitidas.

Em produção, o servidor entra em panic se `MOBILE_JWT_SECRET` ou `MOBILE_REFRESH_SECRET` estiver usando valores default.

## Auditoria Técnica

### OK

- Mobile é encapsulado via `mobile.Wrap(api.Handler())`, sem mexer nas rotas existentes.
- JWT access token é obrigatório nas rotas sensíveis.
- Refresh token é comparado com hash salvo no banco antes de renovar.
- Refresh token é limpo no logout.
- Webhooks mobile têm validação SSRF na criação e o delivery também revalida antes do envio.
- Health check cobre database, price worker, RPC config, event bus e JWT config.
- Produção bloqueia secrets mobile default.

### Gaps Críticos

1. `auth/refresh` falha após registro/login.

Motivo: `SaveRefreshToken` usa bcrypt para hashear o JWT completo. JWT refresh normalmente tem mais de 72 bytes, e bcrypt rejeita senha longa. Os handlers ignoram o erro retornado por `SaveRefreshToken`, então o token não é salvo e `auth/refresh` retorna sessão encerrada.

Correção recomendada:

- salvar SHA-256/HMAC-SHA256 do refresh token em vez de bcrypt; ou
- gerar refresh token opaco curto, com entropia alta, e hashear esse token.

2. `wallet/generate` gera private key no backend e retorna ao app.

Isso funciona para MVP, mas é um risco operacional e regulatório. Produção deveria migrar para geração client-side, wallet custodial formal, MPC, ou fluxo explícito com consentimento e criptografia client-side.

3. Endpoints de contrato expõem comandos sensíveis atrás apenas do JWT mobile.

`/api/mobile/contracts/payout`, `/pause`, `/unpause` não devem ser acessíveis por usuário comum. Mesmo que hoje pareçam stubs/eventos, devem exigir escopo admin/internal ou sair do namespace mobile.

4. Settings ainda são mock.

`GET /settings`, `PUT /settings` e `/settings/limits` retornam dados fixos e não persistem alterações.

5. Idempotência existe mas não está aplicada nas rotas money-moving mobile.

`requireIdempotency` foi implementado, mas as rotas `order/buy`, `order/sell`, `pix/generate`, `swap/execute`, `contracts/payout` não estão envelopadas por ele.

## Testes em Produção

Base testada:

```text
https://stablecoin-payment-gateway-production-3ee2.up.railway.app
```

Comandos foram executados com `curl.exe` no PowerShell usando payload JSON via arquivo ASCII para evitar problemas de quoting.

Resultado observado em 2026-07-13:

- `GET /api/mobile/health`: `200`, ok.
- `GET /api/mobile/assets`: `200`, retornou 6 assets.
- `GET /api/mobile/user/profile` sem token: `401`, ok.
- `POST /api/mobile/auth/register`: `201`, criou usuário e tokens.
- `GET /api/mobile/user/profile` com token: `200`.
- `GET /api/mobile/wallet/address`: `200`, sem wallet gerada.
- `GET /api/mobile/settings`: `200`, dados mock.
- `GET /api/mobile/kyc/limits`: `200`.
- `POST /api/mobile/swap/quote`: `200`, quote USDT->USDC.
- `POST /api/mobile/security/pin` com PIN curto: `400`, validação ok.
- `POST /api/mobile/auth/refresh`: `401`, bug confirmado.
- `POST /api/mobile/auth/logout`: `200`.

## Checklist de Produção

- Corrigir armazenamento/validação de refresh token.
- Remover geração backend de private key ou marcar como MVP/inseguro no produto.
- Colocar endpoints `/contracts/*` sob autorização admin/internal.
- Aplicar idempotência em rotas que criam ordem, swap ou payout.
- Persistir settings de usuário.
- Adicionar testes Go para auth, refresh, SSRF, wallet, KYC e rotas money-moving.
- Adicionar testes E2E mobile com usuário sintético e cleanup.
