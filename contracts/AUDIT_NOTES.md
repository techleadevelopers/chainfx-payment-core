# Audit Notes - Payment Gateway, Signer e Contratos BSC

Data: 2026-07-04

## Escopo

- `C:\Users\Paulo\Desktop\payment-gateway`
- `C:\Users\Paulo\Desktop\payment-gateway\signer`
- nova pasta `contracts/`

## Conclusao

O fluxo atual deve continuar usando o signer Go como camada critica. Os contratos adicionados sao uma opcao de evolucao para treasury/payout on-chain com limites, auditoria e governanca. Eles nao devem ser ligados diretamente em producao sem testnet, auditoria externa e saldo pequeno inicial.

## Pontos Fortes Ja Existentes

- Signer isolado do core.
- HMAC com timestamp e nonce.
- Idempotencia com reserva antes de assinar.
- Custody Guard EIP-7702.
- Lockdown persistente de custodia.
- Nonce manager persistente.
- Lifecycle de transacoes do signer.
- Politica de treasury por saida diaria.
- Restricao de token contract em producao.

## Riscos Encontrados

### Placeholder invalido em delegate confiavel

`CUSTODY_TRUSTED_DELEGATES=0xContratoDelegateSeguro` nao e endereco BSC valido. Foi trocado para vazio em:

- `.env`
- `signer/.env`

Preencher somente depois do deploy e auditoria de um delegate real.

### Contrato on-chain ainda nao integrado ao core

O backend hoje envia por hot wallet via signer. O `SwappyTreasuryVault` esta pronto como camada opcional, mas ainda nao esta plugado no fluxo `BuySendWorker`.

Recomendacao:

1. testar contratos em BSC testnet;
2. criar rota interna/worker para payout via vault;
3. manter fallback manual;
4. limitar saldo inicial;
5. so depois migrar parte do fluxo.

### EIP-7702 delegate exige cautela

`Swappy7702PayoutDelegate` foi escrito sem `execute()` generico. Isso e proposital. Delegate EIP-7702 com chamada arbitraria pode virar dreno total da EOA se houver erro de permissao.

## Contratos Criados

### `SwappyTreasuryVault`

Objetivo: vault de USDT/BEP20 para payout controlado.

Controles:

- owner duas etapas;
- guardian pause;
- operator payout;
- allowlist de token;
- allowlist/blocklist de recipient;
- limite por transferencia;
- limite diario;
- idempotencia por operation id;
- eventos de auditoria.

### `SwappyDelegateRegistry`

Objetivo: governanca on-chain de delegates confiaveis.

O signer Go continua sendo a barreira principal e valida bytecode hash off-chain.

### `Swappy7702PayoutDelegate`

Objetivo: delegate EIP-7702 minimo para payout controlado.

Nao possui execucao arbitraria.

## Plano Seguro de Adoção

1. Rodar `npm install`, `npm run compile`, `npm test` em `contracts/`.
2. Deploy em BSC testnet.
3. Configurar `CUSTODY_MODE=shadow`.
4. Adicionar delegate testnet em `CUSTODY_TRUSTED_DELEGATES`.
5. Validar logs do signer e eventos on-chain.
6. Mudar para `CUSTODY_MODE=paper`.
7. Colocar saldo pequeno no vault.
8. Medir payout real com valor baixo.
9. So entao aumentar limites.

## Regras de Producao

- Owner deve ser multisig ou carteira fria operacional.
- Guardian deve ser separado do owner.
- Operator deve ter limite baixo.
- Limite diario deve ser menor que o saldo total exposto.
- Nunca confiar em delegate sem bytecode hash conferido.
- Nunca usar contrato com `execute()` generico para custody EIP-7702.
