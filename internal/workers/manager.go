package workers

import (
        "context"
        "log/slog"
        "sync"

        "payment-gateway/internal/config"
        "payment-gateway/internal/database"
        "payment-gateway/internal/email"
        "payment-gateway/internal/paymaster"
        "payment-gateway/internal/rpc"
)

type WorkerManager struct {
        Bus                  *EventBus
        PriceWorker          *PriceWorker
        PayoutWorker         *PayoutWorker
        BuySendWorker        *BuySendWorker
        OnchainWorker        *OnchainWorker
        SweepWorker          *SweepWorker
        EmailWorker          *EmailWorker
        M2MSettlementWorker  *M2MSettlementWorker
        AutoSweeperWorker    *AutoSweeperWorker
        PaymasterService     *paymaster.Service
        db                   *database.DB
        cfg                  *config.Config
        wg                   sync.WaitGroup
}

func NewWorkerManager(db *database.DB, cfg *config.Config, mailer *email.Service, pool *rpc.Pool) *WorkerManager {
        bus := NewEventBus()

        var paymasterSvc *paymaster.Service
        if pool != nil {
                paymasterSvc = paymaster.NewService(cfg, db, pool)
        }

        return &WorkerManager{
                Bus:                 bus,
                PriceWorker:         NewPriceWorker(bus),
                PayoutWorker:        NewPayoutWorker(bus, db, cfg),
                BuySendWorker:       NewBuySendWorker(bus, db, cfg),
                OnchainWorker:       NewOnchainWorker(bus, db, cfg),
                SweepWorker:         NewSweepWorker(bus, db, cfg),
                EmailWorker:         NewEmailWorker(bus, db, mailer),
                M2MSettlementWorker: NewM2MSettlementWorker(bus, db, cfg),
                AutoSweeperWorker:   NewAutoSweeperWorker(cfg, db, pool),
                PaymasterService:    paymasterSvc,
                db:                  db,
                cfg:                 cfg,
        }
}

// StartAll starts every worker in its own goroutine.
func (wm *WorkerManager) StartAll(ctx context.Context) {
        slog.Info("Iniciando todos os workers...")

        workerCount := 9 // base 7 + AutoSweeper + Paymaster
        wm.wg.Add(workerCount)

        go func() {
                defer wm.wg.Done()
                wm.PriceWorker.Start(ctx)
        }()

        go func() {
                defer wm.wg.Done()
                wm.PayoutWorker.Start(ctx)
        }()

        go func() {
                defer wm.wg.Done()
                wm.BuySendWorker.Start(ctx)
        }()

        go func() {
                defer wm.wg.Done()
                wm.OnchainWorker.Start(ctx)
        }()

        go func() {
                defer wm.wg.Done()
                wm.SweepWorker.Start(ctx)
        }()

        go func() {
                defer wm.wg.Done()
                wm.EmailWorker.Start(ctx)
        }()

        go func() {
                defer wm.wg.Done()
                wm.M2MSettlementWorker.Start(ctx)
        }()

        go func() {
                defer wm.wg.Done()
                wm.AutoSweeperWorker.Start(ctx)
        }()

        go func() {
                defer wm.wg.Done()
                if wm.PaymasterService != nil {
                        wm.PaymasterService.Start(ctx)
                }
        }()

        slog.Info("Todos os workers iniciados com sucesso", "count", workerCount)
}

// Shutdown aguarda todos os workers finalizarem
func (wm *WorkerManager) Shutdown(ctx context.Context) {
        slog.Info("Iniciando shutdown dos workers...")

        // Fecha o EventBus primeiro para parar de receber novos eventos
        wm.Bus.Shutdown()

        // Aguarda todos os workers finalizarem com timeout
        done := make(chan struct{})
        go func() {
                wm.wg.Wait()
                close(done)
        }()

        select {
        case <-done:
                slog.Info("Todos os workers finalizados com sucesso")
        case <-ctx.Done():
                slog.Warn("Timeout no shutdown dos workers", "timeout", ctx.Err())
        }
}

// StartAllAndWait inicia os workers e aguarda o contexto ser cancelado
func (wm *WorkerManager) StartAllAndWait(ctx context.Context) {
        wm.StartAll(ctx)
        <-ctx.Done()
        wm.Shutdown(context.Background())
}
