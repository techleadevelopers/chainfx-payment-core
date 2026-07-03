package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"payment-gateway/internal/config"
	"payment-gateway/internal/database"
)

type OnchainWorker struct {
	bus    *EventBus
	db     *database.DB
	cfg    *config.Config
	client *http.Client
}

// TronEventResponse mapeia exatamente o JSON retornado pela API de eventos da rede TRON
type TronEventResponse struct {
	Data []struct {
		TransactionID string `json:"transaction_id"`
		BlockNumber   uint64 `json:"block_number"`
		Result        struct {
			To    string `json:"to"`
			Value string `json:"value"`
		} `json:"result"`
	} `json:"data"`
	Meta struct {
		Fingerprint string `json:"fingerprint"`
	} `json:"meta"`
}

func NewOnchainWorker(bus *EventBus, db *database.DB, cfg *config.Config) *OnchainWorker {
	return &OnchainWorker{
		bus: bus,
		db:  db,
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second, // Evita conexões presas na rede TRON
		},
	}
}

func (ow *OnchainWorker) Start(ctx context.Context) {
	slog.Info("OnchainWorker TRON inicializado em background.")

	// Polling a cada 10 segundos idêntico ao Node
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Desligando OnchainWorker de forma segura...")
			return
		case <-ticker.C:
			ow.pollTronEvents(ctx)
		}
	}
}

func (ow *OnchainWorker) pollTronEvents(ctx context.Context) {
	if ow.cfg.TronUsdtContract == "" {
		slog.Warn("TRON_USDT_CONTRACT não configurado; pulando listener on-chain.")
		return
	}
	if ow.cfg.TronFullNodeUrl == "" {
		slog.Warn("TRON_FULLNODE_URL não configurado; pulando listener on-chain.")
		return
	}

	start := time.Now()
	slog.Debug("Iniciando varredura de blocos na TRON...")

	pending, err := ow.db.GetPendingOrders(ctx)
	if err != nil {
		slog.Error("Erro ao buscar ordens pendentes", "error", err)
		return
	}
	if len(pending) == 0 {
		return
	}
	ordersByAddress := make(map[string]struct {
		ID        string
		Expected  float64
		PixCpf    string
		PixPhone  string
		ExpiresAt time.Time
	}, len(pending))
	for _, order := range pending {
		if order.Network == "TRON" || order.Network == "" {
			ordersByAddress[order.Address] = struct {
				ID        string
				Expected  float64
				PixCpf    string
				PixPhone  string
				ExpiresAt time.Time
			}{ID: order.ID, Expected: order.AmountUSDT, PixCpf: order.PixCpf, PixPhone: order.PixPhone, ExpiresAt: order.RateLockExpiresAt}
		}
	}

	url := fmt.Sprintf("%s/v1/contracts/%s/events?event_name=Transfer&only_confirmed=true&limit=50",
		strings.TrimSuffix(ow.cfg.TronFullNodeUrl, "/"),
		ow.cfg.TronUsdtContract,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		slog.Error("Erro ao montar requisição TRON", "error", err)
		return
	}

	resp, err := ow.client.Do(req)
	if err != nil {
		slog.Error("Erro ao consultar API de eventos TRON", "error", err)
		return
	}
	defer resp.Body.Close()

	var result TronEventResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		slog.Error("Erro ao parsear eventos da rede TRON", "error", err)
		return
	}

	// 3. Processa cada transferência encontrada no bloco
	for _, ev := range result.Data {
		// A API da TRON pode retornar endereços em formato Hexadecimal ou Base58 (Tratamos o mapeamento)
		toAddress := ev.Result.To

		order, exists := ordersByAddress[toAddress]
		if !exists {
			continue
		}
		if !order.ExpiresAt.IsZero() && time.Now().After(order.ExpiresAt) {
			_ = ow.db.UpdateOrderStatus(ctx, order.ID, "expirada", map[string]interface{}{"error": "Ordem expirada"})
			continue
		}

		// Converte o valor retornado (Sun) para unidade USDT (6 decimais na rede TRON)
		rawAmount, _ := strconv.ParseFloat(ev.Result.Value, 64)
		amountUSDT := rawAmount / pow10(ow.cfg.TronUsdtDecimals)
		tolerance := ow.cfg.TronDepositTolerancePct
		if tolerance <= 0 {
			tolerance = 0.02
		}
		min := order.Expected * (1 - tolerance)
		max := order.Expected * (1 + tolerance)
		if amountUSDT < min || amountUSDT > max {
			_ = ow.db.UpdateOrderStatus(ctx, order.ID, "aguardando_validacao", map[string]interface{}{"error": "Depósito fora da faixa", "depositTx": ev.TransactionID, "depositAmount": amountUSDT})
			continue
		}
		duplicate, _ := ow.db.HasEvent(ctx, order.ID, "order.pago", "depositTx", ev.TransactionID)
		if duplicate {
			continue
		}

		slog.Info("Depósito detectado na blockchain TRON",
			"order_id", order.ID,
			"address", toAddress,
			"amount_usdt", amountUSDT,
			"tx_hash", ev.TransactionID,
		)
		if err := ow.db.UpdateOrderStatus(ctx, order.ID, "pago", map[string]interface{}{"depositTx": ev.TransactionID, "depositAmount": amountUSDT}); err != nil {
			slog.Error("Erro ao atualizar ordem paga", "order_id", order.ID, "error", err)
			continue
		}

		// 4. Dispara o evento de sucesso de pagamento
		ow.bus.Publish(Event{
			Type:    "onchain.detected",
			OrderID: order.ID,
			Payload: map[string]interface{}{
				"tx_hash":     ev.TransactionID,
				"amount_usdt": amountUSDT,
			},
		})

		// 5. Encaminha automaticamente para a esteira de Payout PIX
		completed, _ := ow.db.CountCompletedOrdersForPix(ctx, order.PixCpf, order.PixPhone)
		if completed == 0 && ow.cfg.OrderHoldSecForNewDest > 0 {
			orderID := order.ID
			delay := time.Duration(ow.cfg.OrderHoldSecForNewDest) * time.Second
			time.AfterFunc(delay, func() {
				ow.bus.Publish(Event{Type: "payout.requested", OrderID: orderID})
			})
		} else {
			ow.bus.Publish(Event{Type: "payout.requested", OrderID: order.ID})
		}
	}

	slog.Info("Ciclo de polling TRON finalizado", "duration_ms", time.Since(start).Milliseconds())
}

func pow10(decimals int) float64 {
	if decimals <= 0 {
		return 1
	}
	out := 1.0
	for i := 0; i < decimals; i++ {
		out *= 10
	}
	return out
}
