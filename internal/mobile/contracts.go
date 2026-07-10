package mobile

import (
	"context"
	"math/big"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"payment-gateway/internal/workers"
)

// handleContractVault — GET /api/mobile/contracts/vault
func (s *Server) handleContractVault(w http.ResponseWriter, r *http.Request) {
	vaultAddr := strings.TrimSpace(s.cfg.TreasuryHot)
	if vaultAddr == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"configured": false,
			"hint":       "set TREASURY_HOT in .env",
		})
		return
	}
	bal, err := fetchBNBBalance(r.Context(), s.cfg.BscRpcUrls, vaultAddr)
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":    true,
		"vault_address": vaultAddr,
		"bnb_balance":   bal,
		"error":         errStr(err),
		"network":       s.cfg.SignerNetwork,
	})
}

// handleContractDelegate — GET /api/mobile/contracts/delegate
func (s *Server) handleContractDelegate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"signer_url":     s.cfg.SignerUrl,
		"signer_network": s.cfg.SignerNetwork,
		"configured":     s.cfg.SignerUrl != "" && s.cfg.SignerHmacSecret != "",
	})
}

// handleContractPayout — POST /api/mobile/contracts/payout
func (s *Server) handleContractPayout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrderID string  `json:"order_id"`
		Amount  float64 `json:"amount"`
		ToAddr  string  `json:"to_address"`
	}
	if err := decodeJSON(r, &req); err != nil || req.OrderID == "" || req.ToAddr == "" || req.Amount <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "order_id, to_address e amount obrigatórios"})
		return
	}
	if s.cfg.SignerUrl == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "signer não configurado", "hint": "set SIGNER_URL in .env"})
		return
	}
	s.workers.Bus.Publish(workers.Event{
		Type:    "mobile.payout.requested",
		OrderID: req.OrderID,
		Payload: map[string]any{"toAddr": req.ToAddr, "amount": req.Amount},
	})
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "order_id": req.OrderID, "status": "payout_enqueued"})
}

// handleContractPause — POST /api/mobile/contracts/pause
func (s *Server) handleContractPause(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":   true,
		"hint": "Pause via signer service — configure SIGNER_URL e TREASURY_HOT",
	})
}

// handleContractUnpause — POST /api/mobile/contracts/unpause
func (s *Server) handleContractUnpause(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":   true,
		"hint": "Unpause via signer service — configure SIGNER_URL e TREASURY_HOT",
	})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func fetchBNBBalance(ctx context.Context, rpcURLs, address string) (string, error) {
	url := strings.Split(rpcURLs, ",")[0]
	if url == "" {
		url = "https://bsc-dataseed.binance.org/"
	}
	client, err := ethclient.DialContext(ctx, url)
	if err != nil {
		return "0", err
	}
	defer client.Close()
	bal, err := client.BalanceAt(ctx, common.HexToAddress(address), nil)
	if err != nil {
		return "0", err
	}
	f := new(big.Float).Quo(new(big.Float).SetInt(bal), big.NewFloat(1e18))
	return f.Text('f', 6), nil
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
