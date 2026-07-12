// Package psp — efi_adapter.go
// EfiAdapter wraps the existing Efí Bank PIX integration behind the Provider interface.
// All heavy lifting (OAuth, certificate loading, charge creation) lives in the
// existing internal/mobile PIX code; this adapter normalises the surface.
package psp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// EfiAdapter implements Provider for Efí Bank PIX.
type EfiAdapter struct {
	clientID     string
	clientSecret string
	pixKey       string
	baseURL      string
	httpClient   *http.Client
}

// NewEfiAdapter creates an EfiAdapter.
// tlsCfg may be nil for development (no mutual TLS).
func NewEfiAdapter(clientID, clientSecret, pixKey, baseURL string, tlsCfg *tls.Config) *EfiAdapter {
	transport := &http.Transport{TLSClientConfig: tlsCfg}
	return &EfiAdapter{
		clientID:     clientID,
		clientSecret: clientSecret,
		pixKey:       pixKey,
		baseURL:      baseURL,
		httpClient:   &http.Client{Transport: transport, Timeout: 15 * time.Second},
	}
}

func (e *EfiAdapter) Name() string { return "efi" }

// CreateCharge creates an immediate PIX charge via Efí Bank /cob endpoint.
func (e *EfiAdapter) CreateCharge(ctx context.Context, charge PixCharge) (*PixChargeResult, error) {
	token, err := e.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("efi: auth: %w", err)
	}

	expiry := charge.ExpirySec
	if expiry <= 0 {
		expiry = 3600
	}

	payload := map[string]any{
		"calendario": map[string]any{"expiracao": expiry},
		"devedor": map[string]any{
			"cpf":  charge.PayerCPF,
			"nome": charge.PayerName,
		},
		"valor":     map[string]any{"original": fmt.Sprintf("%.2f", charge.AmountBRL)},
		"chave":     e.pixKey,
		"solicitacaoPagador": charge.Description,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/v2/cob", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("efi: CreateCharge request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return nil, fmt.Errorf("efi: CreateCharge HTTP %d: %v", resp.StatusCode, errBody)
	}

	var result struct {
		TxID      string `json:"txid"`
		PixCopiaECola string `json:"pixCopiaECola"`
		Location  string `json:"location"`
		Calendario struct {
			Criacao   string `json:"criacao"`
			Expiracao int    `json:"expiracao"`
		} `json:"calendario"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("efi: decode CreateCharge response: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(result.Calendario.Expiracao) * time.Second)
	return &PixChargeResult{
		Provider:     e.Name(),
		TXID:         result.TxID,
		PixCopyPaste: result.PixCopiaECola,
		AmountBRL:    charge.AmountBRL,
		ExpiresAt:    expiresAt,
	}, nil
}

// ParseWebhook parses an Efí Bank PIX webhook notification.
func (e *EfiAdapter) ParseWebhook(_ context.Context, body []byte, _ string) (*PixWebhookPayload, error) {
	var raw struct {
		Pix []struct {
			EndToEndID string `json:"endToEndId"`
			TXID       string `json:"txid"`
			Valor      string `json:"valor"`
			HorarioLiquidacao string `json:"horarioLiquidacao"`
			Pagador    struct {
				Nome  string `json:"nome"`
				Chave string `json:"chave"`
			} `json:"pagador"`
		} `json:"pix"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("efi: parse webhook: %w", err)
	}
	if len(raw.Pix) == 0 {
		return nil, fmt.Errorf("efi: webhook has no pix entries")
	}
	p := raw.Pix[0]
	var amount float64
	fmt.Sscanf(p.Valor, "%f", &amount)
	paidAt, _ := time.Parse(time.RFC3339, p.HorarioLiquidacao)
	return &PixWebhookPayload{
		Provider:   e.Name(),
		TXID:       p.TXID,
		EndToEndID: p.EndToEndID,
		AmountBRL:  amount,
		PaidAt:     paidAt,
		PayerName:  p.Pagador.Nome,
		PayerKey:   p.Pagador.Chave,
	}, nil
}

// HealthCheck calls GET /v2/cob with a non-existent TXID to verify connectivity.
func (e *EfiAdapter) HealthCheck(ctx context.Context) error {
	token, err := e.getToken(ctx)
	if err != nil {
		return fmt.Errorf("efi health: auth failed: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL+"/v2/cob/healthcheck-probe", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	// 404 means the endpoint exists (charge not found) — provider is up.
	if resp.StatusCode == 404 || resp.StatusCode == 200 {
		return nil
	}
	return fmt.Errorf("efi health: unexpected status %d", resp.StatusCode)
}

// getToken fetches a bearer token from Efí Bank OAuth endpoint.
func (e *EfiAdapter) getToken(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/oauth/token", bytes.NewBufferString("grant_type=client_credentials"))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(e.clientID, e.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("efi auth returned %d", resp.StatusCode)
	}

	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}
