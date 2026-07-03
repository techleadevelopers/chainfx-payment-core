package server

import (
	"testing"

	"payment-gateway/internal/config"
)

func TestTransactionFeePercentPlusFixedUsdForBRL(t *testing.T) {
	s := &Server{cfg: &config.Config{FeeBps: 200, FeeFixedUsd: 2}}

	fee := s.transactionFee(100, "BRL", 5)
	if fee != 12 {
		t.Fatalf("expected 12 BRL fee, got %.2f", fee)
	}
}

func TestTransactionFeePercentPlusFixedUsdForUSD(t *testing.T) {
	s := &Server{cfg: &config.Config{FeeBps: 200, FeeFixedUsd: 2}}

	fee := s.transactionFee(100, "USD", 1)
	if fee != 4 {
		t.Fatalf("expected 4 USD fee, got %.2f", fee)
	}
}
