package workers

import (
	"context"
	"payment-gateway/internal/database"
)

type WorkerManager struct {
	Bus         *EventBus
	PriceWorker *PriceWorker
	db          *database.DB
}

func NewWorkerManager(db *database.DB) *WorkerManager {
	bus := NewEventBus()
	return &WorkerManager{
		Bus:         bus,
		PriceWorker: NewPriceWorker(bus),
		db:          db,
	}
}

// StartAll dispara todos os workers em background dentro de Goroutines isoladas
func (wm *WorkerManager) StartAll(ctx context.Context) {
	// A palavra-chave 'go' dispara a função em uma thread leve separada (Goroutine)
	go wm.PriceWorker.Start(ctx)

	// Próximas partes:
	// go wm.OnchainWorker.Start(ctx)
	// go wm.PayoutWorker.Start(ctx)
}
