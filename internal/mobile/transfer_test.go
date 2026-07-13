package mobile

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestParseTokenAmount(t *testing.T) {
	tests := []struct {
		name     string
		amount   string
		decimals int
		want     string
		wantErr  bool
	}{
		{name: "bsc decimals", amount: "1.25", decimals: 18, want: "1250000000000000000"},
		{name: "polygon decimals", amount: "1.25", decimals: 6, want: "1250000"},
		{name: "too many decimals", amount: "0.0000001", decimals: 6, wantErr: true},
		{name: "zero", amount: "0", decimals: 18, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTokenAmount(tt.amount, tt.decimals)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != tt.want {
				t.Fatalf("got %s, want %s", got.String(), tt.want)
			}
		})
	}
}

func TestERC20TransferCalldata(t *testing.T) {
	to := common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
	data := erc20TransferCalldata(to, big.NewInt(1000000))

	if !strings.HasPrefix(data, "0xa9059cbb") {
		t.Fatalf("missing transfer selector: %s", data)
	}
	if len(data) != 138 {
		t.Fatalf("unexpected calldata length %d", len(data))
	}
	if !strings.Contains(strings.ToLower(data), strings.TrimPrefix(strings.ToLower(to.Hex()), "0x")) {
		t.Fatalf("recipient missing from calldata: %s", data)
	}
}
