package paymaster

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// ErrDuplicateSig is returned when an identical EIP-712 (r, s) pair has
// already been processed within the TTL window.
var ErrDuplicateSig = errors.New("paymaster: duplicate signature — relay already submitted")

const sigLockTTL = 2 * time.Minute
const sigLockGCInterval = 30 * time.Second

// SigLockStore is the interface for idempotency stores. Swap in a Redis
// implementation for multi-instance deployments.
type SigLockStore interface {
	// AcquireLock atomically acquires the lock for hash.
	// Returns (true, nil) on first acquisition; (false, ErrDuplicateSig) if already held.
	AcquireLock(hash string) (bool, error)
	// ReleaseLock releases the lock (used only on relay failure to allow retry).
	ReleaseLock(hash string)
}

// PermitSigHash derives the canonical idempotency key from the (r, s) pair of
// an EIP-712 signature. The v component (27 / 28) is intentionally excluded
// to prevent trivial flip-based evasion.
//
// hash = hex(SHA-256(r_bytes || s_bytes))
func PermitSigHash(r, s string) (string, error) {
	rBytes, err := hex.DecodeString(stripHex(r))
	if err != nil {
		return "", err
	}
	sBytes, err := hex.DecodeString(stripHex(s))
	if err != nil {
		return "", err
	}
	combined := append(rBytes, sBytes...) //nolint:gocritic
	sum := sha256.Sum256(combined)
	return hex.EncodeToString(sum[:]), nil
}

func stripHex(s string) string {
	if len(s) >= 2 && s[:2] == "0x" {
		return s[2:]
	}
	return s
}

// ── InMemorySigLock ────────────────────────────────────────────────────────────

type sigEntry struct {
	acquiredAt time.Time
}

// InMemorySigLock is a goroutine-safe in-memory implementation of SigLockStore.
// Suitable for single-instance deployments. For multi-instance, provide a
// Redis-backed implementation via the SigLockStore interface.
type InMemorySigLock struct {
	store sync.Map // hash → *sigEntry
}

// NewInMemorySigLock creates an InMemorySigLock and starts the GC goroutine.
// The GC goroutine exits when ctx is cancelled — pass context.Background() for
// long-lived use.
func NewInMemorySigLock(stopCh <-chan struct{}) *InMemorySigLock {
	sl := &InMemorySigLock{}
	go sl.gc(stopCh)
	return sl
}

func (sl *InMemorySigLock) AcquireLock(hash string) (bool, error) {
	entry := &sigEntry{acquiredAt: time.Now()}
	// LoadOrStore is atomic: only the first caller wins.
	_, loaded := sl.store.LoadOrStore(hash, entry)
	if loaded {
		return false, ErrDuplicateSig
	}
	return true, nil
}

func (sl *InMemorySigLock) ReleaseLock(hash string) {
	sl.store.Delete(hash)
}

func (sl *InMemorySigLock) gc(stopCh <-chan struct{}) {
	ticker := time.NewTicker(sigLockGCInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			now := time.Now()
			sl.store.Range(func(k, v any) bool {
				e, ok := v.(*sigEntry)
				if !ok || now.Sub(e.acquiredAt) > sigLockTTL {
					sl.store.Delete(k)
				}
				return true
			})
		}
	}
}
