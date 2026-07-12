// Package psp defines the Payment Service Provider abstraction layer.
// It allows ChainFX to dynamically route PIX intents between Efí Bank and
// any future backup provider without code changes — only config.
//
// If the active provider's webhook delivery failure rate crosses the threshold
// (detected by the metrics package), the router can automatically promote the
// backup provider without manual intervention, targeting 99.95% availability.
package psp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// PixCharge represents a PIX charge creation request, provider-agnostic.
type PixCharge struct {
	ExternalID  string
	AmountBRL   float64
	Description string
	PayerName   string
	PayerCPF    string // CPF or CNPJ
	ExpirySec   int    // QR code expiry in seconds
}

// PixChargeResult is the provider-agnostic result.
type PixChargeResult struct {
	Provider    string
	ChargeID    string
	TXID        string
	PixCopyPaste string
	QRCodeB64   string
	ExpiresAt   time.Time
	AmountBRL   float64
}

// PixWebhookPayload is what the router delivers to the business layer after
// normalising the provider-specific JSON.
type PixWebhookPayload struct {
	Provider    string
	TXID        string
	EndToEndID  string
	AmountBRL   float64
	PaidAt      time.Time
	PayerName   string
	PayerKey    string
}

// Provider is the core interface every PIX PSP adapter must implement.
type Provider interface {
	// Name returns a short stable identifier, e.g. "efi" or "asaas".
	Name() string

	// CreateCharge creates a PIX QR code charge and returns the details.
	CreateCharge(ctx context.Context, charge PixCharge) (*PixChargeResult, error)

	// ParseWebhook parses and validates the provider-specific webhook body,
	// returning a normalised PixWebhookPayload.
	ParseWebhook(ctx context.Context, body []byte, secret string) (*PixWebhookPayload, error)

	// HealthCheck performs a lightweight liveness probe.
	HealthCheck(ctx context.Context) error
}

// ── Router ────────────────────────────────────────────────────────────────────

// FailureThreshold is the number of consecutive errors before auto-failover.
const FailureThreshold = 5

// Router holds the active + backup providers and performs automatic failover.
type Router struct {
	mu               sync.RWMutex
	active           Provider
	backup           Provider
	consecutiveFails atomic.Int64
	failedOver       atomic.Bool
	failoverAt       time.Time
	lastProbe        time.Time
}

// ErrNoProvider is returned when no provider is configured.
var ErrNoProvider = errors.New("psp: no provider configured")

// NewRouter creates a Router. backup may be nil for single-provider setups.
func NewRouter(active, backup Provider) *Router {
	return &Router{active: active, backup: backup}
}

// CreateCharge routes a charge to the active provider.
// If the active provider fails and a backup is configured, it failovers automatically.
func (rt *Router) CreateCharge(ctx context.Context, charge PixCharge) (*PixChargeResult, error) {
	if rt.active == nil {
		return nil, ErrNoProvider
	}

	result, err := rt.active.CreateCharge(ctx, charge)
	if err == nil {
		rt.consecutiveFails.Store(0)
		return result, nil
	}

	fails := rt.consecutiveFails.Add(1)
	slog.Warn("psp router: active provider error",
		"provider", rt.active.Name(),
		"consecutive_fails", fails,
		"error", err,
	)

	// Auto-failover when threshold is reached.
	if fails >= FailureThreshold && rt.backup != nil && !rt.failedOver.Load() {
		rt.promoteBackup()
	}

	// Retry on backup if available.
	rt.mu.RLock()
	backup := rt.backup
	active := rt.active
	rt.mu.RUnlock()

	if backup != nil && backup.Name() != active.Name() {
		slog.Info("psp router: retrying on backup provider", "provider", backup.Name())
		result, backupErr := backup.CreateCharge(ctx, charge)
		if backupErr == nil {
			return result, nil
		}
		slog.Error("psp router: backup provider also failed",
			"provider", backup.Name(),
			"error", backupErr,
		)
		return nil, fmt.Errorf("all providers failed: primary=%v, backup=%v", err, backupErr)
	}

	return nil, err
}

// ActiveProvider returns the currently active provider name.
func (rt *Router) ActiveProvider() string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	if rt.active == nil {
		return "none"
	}
	return rt.active.Name()
}

// ParseWebhook routes webhook parsing to the named provider.
func (rt *Router) ParseWebhook(ctx context.Context, providerName string, body []byte, secret string) (*PixWebhookPayload, error) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	for _, p := range []Provider{rt.active, rt.backup} {
		if p != nil && p.Name() == providerName {
			return p.ParseWebhook(ctx, body, secret)
		}
	}
	return nil, fmt.Errorf("psp: unknown provider %q", providerName)
}

// ProbeAll runs HealthCheck on all providers and logs results.
func (rt *Router) ProbeAll(ctx context.Context) {
	rt.mu.RLock()
	active := rt.active
	backup := rt.backup
	rt.mu.RUnlock()

	for _, p := range []Provider{active, backup} {
		if p == nil {
			continue
		}
		if err := p.HealthCheck(ctx); err != nil {
			slog.Warn("psp health check failed", "provider", p.Name(), "error", err)
		} else {
			slog.Debug("psp health check ok", "provider", p.Name())
		}
	}

	// After a failover, periodically attempt to restore the original active.
	if rt.failedOver.Load() && time.Since(rt.failoverAt) > 5*time.Minute {
		rt.tryRestoreActive(ctx, active)
	}
}

func (rt *Router) promoteBackup() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.backup == nil {
		return
	}
	slog.Warn("psp router: auto-failover triggered",
		"from", rt.active.Name(),
		"to", rt.backup.Name(),
	)
	rt.active, rt.backup = rt.backup, rt.active
	rt.failedOver.Store(true)
	rt.failoverAt = time.Now()
	rt.consecutiveFails.Store(0)
}

func (rt *Router) tryRestoreActive(ctx context.Context, original Provider) {
	if original == nil {
		return
	}
	if err := original.HealthCheck(ctx); err != nil {
		slog.Debug("psp router: original provider still unhealthy, staying on backup",
			"provider", original.Name(), "error", err)
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	// Swap back: current active (backup) becomes backup again.
	rt.active, rt.backup = original, rt.active
	rt.failedOver.Store(false)
	rt.consecutiveFails.Store(0)
	slog.Info("psp router: restored original provider", "provider", original.Name())
}
