<div align="center\">
<img src="https://res.cloudinary.com/limpeja/image/upload/v1770993671/swap_1_mvctri.png" alt="Swappy Logo" width="320">
<h3>Swappy Financial Core </h3>
<p>Infraestrutura de Alta Performance para On/Off-Ramp Automatizado de Criptoativos (USDT/BRL)</p>
</div>

---

##  1. Visão Arquitetural do Ecossistema

O **Swappy Financial Core** é uma stack transacional de nível industrial desenhada especificamente para operações de liquidação instantânea de criptoativos (*Sell/Off-ramp* e *Buy/On-ramp*). O sistema foi arquitetado sob o padrão de **Monorepo em Go**, separando estritamente a API pública de I/O, os Workers assíncronos orientados a eventos e o Cofre Criptográfico de Assinaturas (`signer`).

### Divisão de Responsabilidades (Isolamento de Processos)
1. **API Gateway Core (`cmd/api`):** Camada de entrada pública, enxuta e endurecida. Responsável por expor endpoints REST, aplicar Rate Limiting, travar cotações com TTL estrito via cache em memória e persistir intenções de ordens no PostgreSQL com status `aguardando_deposito`.
2. **Asynchronous Processing Workers (`internal/workers`):** Daemons assíncronos isolados que escutam um Barramento de Eventos de memória (com interface abstrata pronta para acoplamento em filas gerenciadas como AWS SQS ou Apache Kafka).
   * **`PriceWorker`:** Sincroniza e faz o cache da cotação institucional com TTL controlado.
   * **`OnchainWorker`:** Escuta ativamente os nós de RPC (TRON/BSC), processa eventos de blocos confirmados e valida se os depósitos dos usuários entraram com as confirmações matemáticas exigidas.
   * **`PayoutWorker`:** Conecta-se às APIs bancárias reguladas (PIX/PagBank) para executar a liquidação em moeda fiduciária instantaneamente após a validação cripto.
   * **`SweepWorker`:** Varre os endereços efêmeros de depósito dos usuários (*Child Addresses*) enviando os fundos para a carteira fria/tesouraria central.
3. **Cofre Isolado Signer (`signer/`):** Microsserviço crítico rodando em sub-rede privada (*Air-gapped* lógico). É o único processo que retém as chaves privadas (`EVM_PRIVATE_KEY` / `TRON_XPRV`). Nenhuma outra parte do sistema tem acesso à memória onde as chaves operam.

---

##  2. Porquê Go? (Decisões de Engenharia de Produção)

A infraestrutura original em Node.js (Express) apresentava gargalos críticos para a escala financeira real que foram mitigados pela migração para Go:

* **Gerenciamento de CPU-Bound vs I/O-Bound:** A validação e geração de assinaturas criptográficas (HMAC-SHA256 e Criptografia de Curva Elíptica ECDSA) são operações intensivas de CPU. No Node.js, isso bloqueava o *Event Loop Single-Threaded*, atrasando requisições HTTP de entrada. O Go resolve isto nativamente escalonando Goroutines entre múltiplos cores de CPU via *M:N Scheduler*.
* **Segurança e Imutabilidade de Memória:** Strings em Node/V8 podem sofrer vazamento em buffers compartilhados em caso de *memory dumping* após falhas catastróficas. O Go oferece tipagem estática e total controle sobre ponteiros e arrays de bytes (`[]byte`), permitindo que dados sensíveis de chaves e buffers de criptografia sejam limpos da memória de forma previsível e segura.
* **Race Detector Nativo:** Sistemas de criptografia que gerenciam saldos concorrentes não podem sofrer de *Race Conditions*. O compilador do Go traz a flag `-race`, utilizada em nossas esteiras de CI/CD para auditar matematicamente se duas threads tentaram atualizar ou liquidar a mesma ordem ao mesmo tempo.

---

## 🔐 3. Engenharia de Segurança e Equações Matemáticas

### O Escudo HMAC-SHA256 Ponta a Ponta
Para impedir interceptações na rede interna, qualquer comunicação da API Core em direção ao `/hd/transfer` do `signer` é assinada usando uma equação de criptografia simétrica:

$$\text{Digest} = \text{HMAC-SHA256}\Big(\text{HMAC\_SECRET}, \; \text{x-ts} \parallel \text{"."} \parallel \text{x-nonce} \parallel \text{"."} \parallel \text{RawBody}\Big)$$

Onde:
* $\parallel$ representa a concatenação binária exata dos componentes.
* `x-ts` é o Unix Timestamp da requisição. O Signer rejeita requisições onde $| \text{Tempo\_Atual} - \text{x-ts} | > 60\text{s}$, eliminando **Ataques de Replay**.
* `x-nonce` é um identificador único de 16 caracteres. O Signer salva cada nonce no banco com índice `UNIQUE`. Se o mesmo nonce for enviado duas vezes na janela válida de tempo, a transação sofre um *abort* imediato no banco de dados.

### Derivação de Carteiras Determinísticas (Padrão BIP44)
O sistema não gera um endereço estático para os usuários depositarem, evitando correlação pública de balanço. O `SweepWorker` utiliza a chave estendida privada (`TRON_XPRV`) para derivar caminhos matemáticos exclusivos por usuário:

$$\text{Endereço} = \text{Derivar}\big(\text{XPRV}, \; m/44'/195'/0'/0/\text{index}\big)$$

Isso permite gerar bilhões de endereços de depósitos monitorados pelo `OnchainWorker`, mantendo o controle centralizado sob uma única semente (*Seed Master*).

---

##  4. Ciclo de Vida Transacional (Idempotência Célula-Mãe)

Para evitar o pior cenário de um gateway financeiro — o **Duplo Gasto** ou **Dupla Liquidação** (enviar dois PIX para o mesmo depósito ou assinar duas transferências on-chain por instabilidade de rede), implementamos o padrão de **Idempotência Persistida**.