package liquidity

import (
	"context"
	"errors"
	"testing"
)

type quoteProvider struct {
	name  string
	quote Quote
	err   error
}

func (p quoteProvider) Name() string { return p.name }

func (p quoteProvider) Quote(context.Context, Request) (Quote, error) {
	if p.err != nil {
		return Quote{}, p.err
	}
	return p.quote, nil
}

type executableProvider struct {
	quoteProvider
	exec Execution
}

func (p executableProvider) Execute(context.Context, Request, Quote) (Execution, error) {
	return p.exec, nil
}

func TestRouterSelectsLowestNetCostWithSLA(t *testing.T) {
	router := NewRouter(
		quoteProvider{name: "slow-cheap", quote: Quote{TotalCostBRL: 100, CryptoAmount: 0.001, DeliverySLASeconds: 1200, ReliabilityBps: 9000}},
		quoteProvider{name: "fast-fair", quote: Quote{TotalCostBRL: 101, CryptoAmount: 0.001, DeliverySLASeconds: 60, ReliabilityBps: 9800}},
		quoteProvider{name: "broken", err: errors.New("down")},
	)

	best, quotes, err := router.BestQuote(context.Background(), Request{
		OrderID:         "buy-1",
		Asset:           "btc",
		Network:         "bitcoin",
		AmountBRL:       110,
		CryptoAmount:    0.001,
		QuoteLockedRate: 100000,
	})
	if err != nil {
		t.Fatalf("BestQuote returned error: %v", err)
	}
	if len(quotes) != 2 {
		t.Fatalf("expected 2 usable quotes, got %d", len(quotes))
	}
	if best.Provider != "fast-fair" {
		t.Fatalf("expected fast-fair to win after SLA penalty, got %s", best.Provider)
	}
	if best.Asset != "BTC" || best.Network != "BITCOIN" {
		t.Fatalf("quote was not normalized: %+v", best)
	}
}

func TestRouterExecuteBestRequiresExecutableProvider(t *testing.T) {
	router := NewRouter(quoteProvider{name: "quote-only", quote: Quote{TotalCostBRL: 100, CryptoAmount: 0.001}})

	best, quotes, _, err := router.ExecuteBest(context.Background(), Request{
		OrderID:         "buy-1",
		Asset:           "BTC",
		Network:         "BITCOIN",
		AmountBRL:       100,
		CryptoAmount:    0.001,
		QuoteLockedRate: 100000,
	})
	if !errors.Is(err, ErrNoExecutable) {
		t.Fatalf("expected ErrNoExecutable, got %v", err)
	}
	if best.Provider != "quote-only" || len(quotes) != 1 {
		t.Fatalf("expected selected quote to be returned for audit, best=%+v quotes=%d", best, len(quotes))
	}
}

func TestRouterExecuteBestReturnsExecution(t *testing.T) {
	router := NewRouter(executableProvider{
		quoteProvider: quoteProvider{name: "partner-a", quote: Quote{TotalCostBRL: 100, CryptoAmount: 0.001}},
		exec:          Execution{Status: "sent", TxHash: "0xabc"},
	})

	best, _, exec, err := router.ExecuteBest(context.Background(), Request{
		OrderID:         "buy-1",
		Asset:           "BTC",
		Network:         "BITCOIN",
		AmountBRL:       100,
		CryptoAmount:    0.001,
		QuoteLockedRate: 100000,
	})
	if err != nil {
		t.Fatalf("ExecuteBest returned error: %v", err)
	}
	if best.Provider != "partner-a" || exec.Provider != "partner-a" || exec.TxHash != "0xabc" {
		t.Fatalf("unexpected execution: best=%+v exec=%+v", best, exec)
	}
}
