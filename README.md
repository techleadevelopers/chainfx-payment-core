# Swappy Payment Gateway

Backend Go para orquestracao instantanea de settlement fiat -> USDT.

Swappy opera como um **instant settlement orchestration system**: recebe fiat por rails tradicionais, confirma o pagamento, registra tudo de forma auditavel e dispara entrega cripto para a wallet do usuario.

## Indice

1. [Sobre o Swappy](#sobre-o-swappy)
2. [Fluxo do Cliente](#fluxo-do-cliente)
3. [Principais Capacidades](#principais-capacidades)
4. [Arquitetura Tecnica](#arquitetura-tecnica)
5. [Deploy](#deploy)
6. [Documentacao Tecnica](#documentacao-tecnica)
7. [Licenca](#licenca)

## Sobre o Swappy

Swappy permite comprar e vender stablecoins como USDT de forma rapida, com integracao via PIX e entrega cripto para wallet TRON.

Principais diferenciais:

- Compra de USDT via PIX.
- Cotacao travada por janela configuravel.
- Webhook de pagamento com HMAC.
- Delivery cripto assinado por signer isolado.
- Auditoria por ordem, request ID, provider ID e hash on-chain.
- LGPD por minimizacao, hash e criptografia AES-GCM nos dados sensiveis de SELL.

## Fluxo do Cliente

### BUY BRL via Pix

1. Usuario informa quanto quer pagar em BRL.
2. API retorna cotacao e quantidade estimada de USDT.
3. Usuario informa wallet TRON.
4. Gateway cria `buy_order` em `aguardando_pix`.
5. Cliente paga o PIX.
6. Webhook bancario confirma pagamento.
7. Gateway marca `pago_fiat`.
8. `BuySendWorker` entrega USDT para a wallet do cliente.
9. Ordem recebe `tx_hash_out` e `delivered_at`.

Fluxo esperado:

```text
Cliente paga Pix -> Webhook confirma -> BuySendWorker dispara da wallet Swappy -> USDT chega na wallet do cliente
```

### SELL USDT -> Pix

1. Usuario informa chave PIX e valor BRL.
2. Gateway gera endereco de deposito TRON deterministico.
3. Monitor on-chain confirma deposito USDT.
4. `PayoutWorker` liquida PIX para o usuario.

## Principais Capacidades

- API publica em `cmd/api`.
- Workers concorrentes em `internal/workers`.
- Persistencia PostgreSQL em `internal/database`.
- Webhooks PIX e Stripe com idempotencia.
- SSE para acompanhamento de status.
- Healthcheck `/healthz` e readiness `/readyz`.
- Benchmark do fluxo PIX -> delivery em `cmd/benchflow`.
- Deploy por `Dockerfile` e `railway.json`.

## Arquitetura Tecnica

A documentacao tecnica completa esta em [ARCHITECTURE.md](./ARCHITECTURE.md).

Ela cobre:

- Diagrama de sequencia.
- Componentes internos.
- Endpoints.
- Status de ordens.
- Webhooks.
- Variaveis de ambiente.
- Benchmark E2E.
- Deploy Railway/Docker.
- Troubleshooting.
- Rollback operacional.

## Deploy

Este repositorio inclui:

- `Dockerfile`
- `.dockerignore`
- `railway.json`

No Railway, configure as variaveis de ambiente de producao antes do deploy:

```env
APP_ENV=production
ALLOW_SIMULATIONS=false
PORT=3000
DATABASE_URL=postgres://...
LGPD_SECRET=...
WEBHOOK_SECRET=...
PIX_WEBHOOK_SECRET=...
PAGSEGURO_API_TOKEN=...
SIGNER_URL=...
SIGNER_NETWORK=tron
SIGNER_HMAC_SECRET=...
TRON_XPUB=...
TRON_USDT_CONTRACT=...
TRON_FULLNODE_URL=...
TREASURY_HOT=...
```

Mais detalhes em [ARCHITECTURE.md](./ARCHITECTURE.md#deploy).

## Documentacao Tecnica

- [ARCHITECTURE.md](./ARCHITECTURE.md): especificacao tecnica e operacional.
- [schema.sql](./schema.sql): estrutura SQL.
- [signer/README.md](./signer/README.md): signer isolado.

## Licenca

Licenca ainda nao definida neste repositorio. Antes de distribuicao publica, adicionar um arquivo `LICENSE` com a licenca escolhida.
