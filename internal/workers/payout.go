package workers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"payment-gateway/internal/config"
	"payment-gateway/internal/database"
	"payment-gateway/internal/httpclient"
)

type PayoutWorker struct {
	bus    *EventBus
	db     *database.DB
	cfg    *config.Config
	client *http.Client
}

func NewPayoutWorker(bus *EventBus, db *database.DB, cfg *config.Config) *PayoutWorker {
	return &PayoutWorker{
		bus:    bus,
		db:     db,
		cfg:    cfg,
		client: httpclient.Default(),
	}
}

func (pw *PayoutWorker) Start(ctx context.Context) {
	payoutChan := pw.bus.Subscribe("payout.requested")
	slog.Info("PayoutWorker escutando eventos 'payout.requested'")

	for {
		select {
		case <-ctx.Done():
			slog.Info("Desligando PayoutWorker")
			return
		case event, ok := <-payoutChan:
			if !ok {
				return
			}
			go pw.processPayout(event)
		}
	}
}

func (pw *PayoutWorker) processPayout(event Event) {
	start := time.Now()
	orderID := event.OrderID

	slog.Info("Processando Payout PIX", "order_id", orderID)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	order, err := pw.db.GetOrder(ctx, orderID)
	if err != nil {
		slog.Error("Erro ao buscar ordem para payout", "order_id", orderID, "error", err)
		return
	}
	if order == nil || string(order.Status) != "pago" {
		return
	}

	if pw.cfg.AllowSimulations && !pw.cfg.IsProduction() {
		txHash := fmt.Sprintf("pix-sim-%s", orderID)
		if err := pw.db.UpdateOrderStatus(ctx, orderID, "concluida", map[string]interface{}{"txHash": txHash}); err != nil {
			slog.Error("Erro ao persistir payout simulado", "order_id", orderID, "error", err)
			return
		}
		pw.bus.Publish(Event{Type: "payout.settled", OrderID: orderID, Payload: map[string]interface{}{"status": "concluida", "tx_hash_pix": txHash}})
		slog.Warn("Payout PIX simulado concluido", "order_id", orderID, "duration_ms", time.Since(start).Milliseconds())
		return
	}
	_ = pw.db.UpdateOrderStatus(ctx, orderID, "erro", map[string]interface{}{"error": "Efí Pix cash-out nao implementado"})
	slog.Error("Payout PIX bloqueado: Efí cash-out ainda nao implementado", "order_id", orderID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "chave@pix.com"
}
