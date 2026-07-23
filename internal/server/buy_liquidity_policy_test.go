package server

import "testing"

func TestValidBuyDeliveryAddressUsesStrictSolanaValidation(t *testing.T) {
	valid := "11111111111111111111111111111111"
	if !validBuyDeliveryAddress("SOLANA", valid) {
		t.Fatalf("expected 32-byte Solana public key to be accepted")
	}

	invalid := "111111111111111111111111111111111"
	if validBuyDeliveryAddress("SOLANA", invalid) {
		t.Fatalf("expected base58 address with invalid decoded length to be rejected")
	}
}
