package database

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrNFCIdempotencyPayloadMismatch = errors.New("nfc: idempotency payload mismatch")

const (
	NFCStatusApproved        = "approved"
	NFCStatusDeclined        = "declined"
	NFCStatusRequiresFunding = "requires_funding"
	NFCStatusCaptured        = "captured"
	NFCStatusReversed        = "reversed"
	NFCStatusExpired         = "expired"
)

const (
	MerchantSettlementStatusPending   = "PENDING"
	MerchantSettlementStatusSubmitted = "SUBMITTED"
	MerchantSettlementStatusConfirmed = "CONFIRMED"
	MerchantSettlementStatusFailed    = "FAILED"
)

type NFCTokenInput struct {
	TokenID   string
	TokenHash string
	Wallet    string
	DeviceID  string
	Network   string
	ExpiresAt time.Time
}

type NFCFundingInput struct {
	Wallet     string
	Network    string
	Asset      string
	DeltaMicro int64
}

type NFCAuthorizeInput struct {
	ID              string
	IdempotencyKey  string
	TokenID         string
	TokenHash       string
	Wallet          string
	Network         string
	MerchantID      string
	TerminalID      string
	ExternalRef     string
	AmountBRLMinor  int64
	FeeBRLMinor     int64
	TotalBRLMinor   int64
	FeeBps          int
	USDTRate        float64
	RequiredUSDTMic int64
	HoldExpiresAt   time.Time
}

type NFCAuthorization struct {
	ID              string     `json:"id"`
	IdempotencyKey  string     `json:"-"`
	TokenID         string     `json:"token_id"`
	Wallet          string     `json:"wallet_address"`
	Network         string     `json:"network"`
	MerchantID      string     `json:"merchant_id"`
	TerminalID      string     `json:"terminal_id"`
	ExternalRef     string     `json:"external_ref,omitempty"`
	AmountBRLMinor  int64      `json:"amount_brl_minor"`
	FeeBRLMinor     int64      `json:"fee_brl_minor"`
	TotalBRLMinor   int64      `json:"total_brl_minor"`
	FeeBps          int        `json:"fee_bps"`
	USDTRate        float64    `json:"usdt_rate"`
	RequiredUSDTMic int64      `json:"required_usdt_micro"`
	Status          string     `json:"status"`
	ResponseCode    string     `json:"response_code"`
	Reason          string     `json:"reason,omitempty"`
	HoldExpiresAt   *time.Time `json:"hold_expires_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Idempotent      bool       `json:"idempotent,omitempty"`
}

type MerchantSettlement struct {
	ID                string     `json:"id"`
	MerchantID        string     `json:"merchant_id"`
	TerminalID        string     `json:"terminal_id"`
	AuthorizationID   string     `json:"authorization_id"`
	CaptureID         string     `json:"capture_id"`
	AmountBRLMinor    int64      `json:"amount_brl_minor"`
	FeeBRLMinor       int64      `json:"fee_brl_minor"`
	Provider          string     `json:"provider"`
	Rail              string     `json:"rail"`
	Status            string     `json:"status"`
	ProviderReference string     `json:"provider_reference,omitempty"`
	ProviderStatus    string     `json:"provider_status,omitempty"`
	TXID              string     `json:"txid,omitempty"`
	IdempotencyKey    string     `json:"idempotency_key"`
	TargetPixKey      string     `json:"target_pix_key,omitempty"`
	TargetDocument    string     `json:"target_document,omitempty"`
	RetryCount        int        `json:"retry_count"`
	NextRetryAt       time.Time  `json:"next_retry_at"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	SubmittedAt       *time.Time `json:"submitted_at,omitempty"`
	ConfirmedAt       *time.Time `json:"confirmed_at,omitempty"`
	FailedAt          *time.Time `json:"failed_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type NFCCaptureResult struct {
	Authorization *NFCAuthorization   `json:"authorization"`
	Settlement    *MerchantSettlement `json:"settlement,omitempty"`
}

type NFCBalance struct {
	Wallet         string    `json:"wallet_address"`
	Network        string    `json:"network"`
	Asset          string    `json:"asset"`
	AvailableMicro int64     `json:"available_usdt_micro"`
	LockedMicro    int64     `json:"locked_usdt_micro"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type NFCTerminalPolicy struct {
	MerchantID         string `json:"merchant_id"`
	TerminalID         string `json:"terminal_id"`
	MerchantStatus     string `json:"merchant_status"`
	TerminalStatus     string `json:"terminal_status"`
	MaxAmountBRLMinor  int64  `json:"max_amount_brl_minor"`
	DailyLimitBRLMinor int64  `json:"daily_limit_brl_minor"`
	RiskPolicyVersion  string `json:"risk_policy_version"`
	SettlementPixKey   string `json:"settlement_pix_key,omitempty"`
	SettlementDocument string `json:"settlement_document,omitempty"`
}

type NFCTerminalSeed struct {
	MerchantID         string
	TerminalID         string
	APIKey             string
	MerchantName       string
	MaxAmountBRLMinor  int64
	DailyLimitBRLMinor int64
}

func (db *DB) SeedNFCTerminals(ctx context.Context, spec string) error {
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.Split(item, ":")
		if len(parts) < 3 {
			return fmt.Errorf("nfc: invalid NFC_TERMINALS entry")
		}
		in := NFCTerminalSeed{
			MerchantID:   strings.TrimSpace(parts[0]),
			TerminalID:   strings.TrimSpace(parts[1]),
			APIKey:       strings.TrimSpace(parts[2]),
			MerchantName: strings.TrimSpace(parts[0]),
		}
		if len(parts) >= 4 {
			in.MerchantName = strings.TrimSpace(parts[3])
		}
		if err := db.UpsertNFCTerminal(ctx, in); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) UpsertNFCTerminal(ctx context.Context, in NFCTerminalSeed) error {
	merchantID := strings.TrimSpace(in.MerchantID)
	terminalID := strings.TrimSpace(in.TerminalID)
	apiKey := strings.TrimSpace(in.APIKey)
	if merchantID == "" || terminalID == "" || apiKey == "" {
		return fmt.Errorf("nfc: merchant_id, terminal_id and api key are required")
	}
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, `
INSERT INTO nfc_merchants (id, display_name, status)
VALUES ($1,$2,'active')
ON CONFLICT (id) DO UPDATE SET display_name=EXCLUDED.display_name, updated_at=NOW()`,
		merchantID, firstNonEmptyDB(in.MerchantName, merchantID)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO nfc_terminals
  (id, merchant_id, api_key_hash, status, max_amount_brl_minor, daily_limit_brl_minor)
VALUES ($1,$2,$3,'active',$4,$5)
ON CONFLICT (merchant_id, id) DO UPDATE SET
  api_key_hash=EXCLUDED.api_key_hash,
  status='active',
  max_amount_brl_minor=EXCLUDED.max_amount_brl_minor,
  daily_limit_brl_minor=EXCLUDED.daily_limit_brl_minor,
  updated_at=NOW()`,
		terminalID, merchantID, nfcAPIKeyHash(apiKey), in.MaxAmountBRLMinor, in.DailyLimitBRLMinor); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) ValidateNFCTerminal(ctx context.Context, merchantID, terminalID, apiKey string) (*NFCTerminalPolicy, error) {
	merchantID = strings.TrimSpace(merchantID)
	terminalID = strings.TrimSpace(terminalID)
	apiKey = strings.TrimSpace(apiKey)
	if merchantID == "" || terminalID == "" || apiKey == "" {
		return nil, nil
	}
	const q = `
SELECT m.id, t.id, t.api_key_hash, m.status, t.status, t.max_amount_brl_minor, t.daily_limit_brl_minor,
       t.risk_policy_version, COALESCE(m.settlement_pix_key,''), COALESCE(m.settlement_document,'')
FROM nfc_terminals t
JOIN nfc_merchants m ON m.id = t.merchant_id
WHERE t.merchant_id = $1 AND t.id = $2`
	var p NFCTerminalPolicy
	var storedHash string
	err := db.SQL.QueryRowContext(ctx, q, merchantID, terminalID).Scan(
		&p.MerchantID, &p.TerminalID, &storedHash, &p.MerchantStatus, &p.TerminalStatus,
		&p.MaxAmountBRLMinor, &p.DailyLimitBRLMinor, &p.RiskPolicyVersion,
		&p.SettlementPixKey, &p.SettlementDocument,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	gotHash := nfcAPIKeyHash(apiKey)
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(gotHash)) != 1 {
		return nil, nil
	}
	return &p, nil
}

func nfcAPIKeyHash(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:])
}

func (db *DB) StoreNFCToken(ctx context.Context, in NFCTokenInput) error {
	_, err := db.SQL.ExecContext(ctx, `
INSERT INTO nfc_tokens (token_id, token_hash, wallet_address, device_id, network, status, expires_at)
VALUES ($1,$2,$3,$4,$5,'active',$6)
ON CONFLICT (token_id) DO UPDATE SET
  token_hash=EXCLUDED.token_hash,
  wallet_address=EXCLUDED.wallet_address,
  device_id=EXCLUDED.device_id,
  network=EXCLUDED.network,
  status='active',
  expires_at=EXCLUDED.expires_at`,
		strings.TrimSpace(in.TokenID),
		strings.TrimSpace(in.TokenHash),
		strings.ToLower(strings.TrimSpace(in.Wallet)),
		nullableString(strings.TrimSpace(in.DeviceID)),
		normalizeNFCNetwork(in.Network),
		in.ExpiresAt.UTC(),
	)
	return err
}

func (db *DB) AddNFCBalance(ctx context.Context, in NFCFundingInput) (*NFCBalance, error) {
	if in.DeltaMicro <= 0 {
		return nil, fmt.Errorf("nfc: funding delta must be positive")
	}
	asset := strings.ToUpper(firstNonEmptyDB(in.Asset, "USDT"))
	network := normalizeNFCNetwork(in.Network)
	wallet := strings.ToLower(strings.TrimSpace(in.Wallet))
	const q = `
INSERT INTO nfc_wallet_balances (wallet_address, network, asset, available_usdt_micro, locked_usdt_micro)
VALUES ($1,$2,$3,$4,0)
ON CONFLICT (wallet_address, network, asset) DO UPDATE SET
  available_usdt_micro = nfc_wallet_balances.available_usdt_micro + EXCLUDED.available_usdt_micro,
  updated_at = NOW()
RETURNING wallet_address, network, asset, available_usdt_micro, locked_usdt_micro, updated_at`
	return scanNFCBalance(db.SQL.QueryRowContext(ctx, q, wallet, network, asset, in.DeltaMicro))
}

func (db *DB) GetNFCBalance(ctx context.Context, wallet, network string) (*NFCBalance, error) {
	const q = `
SELECT wallet_address, network, asset, available_usdt_micro, locked_usdt_micro, updated_at
FROM nfc_wallet_balances
WHERE wallet_address = $1 AND network = $2 AND asset = 'USDT'`
	bal, err := scanNFCBalance(db.SQL.QueryRowContext(ctx, q, strings.ToLower(strings.TrimSpace(wallet)), normalizeNFCNetwork(network)))
	if err == sql.ErrNoRows {
		return &NFCBalance{Wallet: strings.ToLower(strings.TrimSpace(wallet)), Network: normalizeNFCNetwork(network), Asset: "USDT"}, nil
	}
	return bal, err
}

func (db *DB) AuthorizeNFCPayment(ctx context.Context, in NFCAuthorizeInput) (*NFCAuthorization, bool, error) {
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("nfc: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if existing, err := txGetNFCAuthorizationByIdempotency(ctx, tx, in.TerminalID, in.IdempotencyKey); err != nil {
		return nil, false, err
	} else if existing != nil {
		if !sameNFCAuthorizationPayload(existing, in) {
			return nil, false, ErrNFCIdempotencyPayloadMismatch
		}
		existing.Idempotent = true
		return existing, true, tx.Commit()
	}

	status := NFCStatusDeclined
	responseCode := "05"
	reason := "invalid_token"
	var holdExpires any

	var dbWallet, dbNetwork, tokenStatus string
	var tokenExpires time.Time
	err = tx.QueryRowContext(ctx, `
SELECT wallet_address, network, status, expires_at
FROM nfc_tokens
WHERE token_id = $1 AND token_hash = $2
FOR UPDATE`, in.TokenID, in.TokenHash).Scan(&dbWallet, &dbNetwork, &tokenStatus, &tokenExpires)
	if err == sql.ErrNoRows {
		return txInsertNFCAuthorization(ctx, tx, in, status, responseCode, reason, holdExpires)
	}
	if err != nil {
		return nil, false, fmt.Errorf("nfc: token lookup: %w", err)
	}
	if tokenStatus != "active" || !time.Now().UTC().Before(tokenExpires.UTC()) {
		reason = "token_expired_or_revoked"
		return txInsertNFCAuthorization(ctx, tx, in, status, responseCode, reason, holdExpires)
	}
	if strings.ToLower(dbWallet) != strings.ToLower(in.Wallet) || normalizeNFCNetwork(dbNetwork) != normalizeNFCNetwork(in.Network) {
		reason = "token_wallet_mismatch"
		return txInsertNFCAuthorization(ctx, tx, in, status, responseCode, reason, holdExpires)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE nfc_tokens
SET status = 'revoked', last_used_at = NOW()
WHERE token_id = $1 AND status = 'active'`, in.TokenID); err != nil {
		return nil, false, fmt.Errorf("nfc: consume token: %w", err)
	}

	var available, locked int64
	err = tx.QueryRowContext(ctx, `
SELECT available_usdt_micro, locked_usdt_micro
FROM nfc_wallet_balances
WHERE wallet_address = $1 AND network = $2 AND asset = 'USDT'
FOR UPDATE`, strings.ToLower(in.Wallet), normalizeNFCNetwork(in.Network)).Scan(&available, &locked)
	if err == sql.ErrNoRows || available < in.RequiredUSDTMic {
		status = NFCStatusRequiresFunding
		responseCode = "51"
		reason = "insufficient_usdt"
		return txInsertNFCAuthorization(ctx, tx, in, status, responseCode, reason, holdExpires)
	}
	if err != nil {
		return nil, false, fmt.Errorf("nfc: balance lookup: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
UPDATE nfc_wallet_balances
SET available_usdt_micro = available_usdt_micro - $3,
    locked_usdt_micro = locked_usdt_micro + $3,
    updated_at = NOW()
WHERE wallet_address = $1 AND network = $2 AND asset = 'USDT'`,
		strings.ToLower(in.Wallet), normalizeNFCNetwork(in.Network), in.RequiredUSDTMic)
	if err != nil {
		return nil, false, fmt.Errorf("nfc: lock balance: %w", err)
	}

	status = NFCStatusApproved
	responseCode = "00"
	reason = "approved"
	holdExpires = in.HoldExpiresAt.UTC()
	return txInsertNFCAuthorization(ctx, tx, in, status, responseCode, reason, holdExpires)
}

func (db *DB) GetNFCAuthorization(ctx context.Context, id string) (*NFCAuthorization, error) {
	const q = `
SELECT id, idempotency_key, token_id, wallet_address, network, merchant_id, terminal_id, COALESCE(external_ref,''),
       amount_brl_minor, COALESCE(fee_brl_minor,0), COALESCE(total_brl_minor, amount_brl_minor), COALESCE(fee_bps,0),
       usdt_rate::float8, required_usdt_micro, status, response_code, COALESCE(reason,''),
       hold_expires_at, created_at, updated_at
FROM nfc_authorizations
WHERE id = $1`
	auth, err := scanNFCAuthorization(db.SQL.QueryRowContext(ctx, q, strings.TrimSpace(id)))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return auth, err
}

func (db *DB) CaptureNFCAuthorization(ctx context.Context, id string) (*NFCCaptureResult, error) {
	return db.captureNFCAuthorization(ctx, id)
}

func (db *DB) ReverseNFCAuthorization(ctx context.Context, id string) (*NFCAuthorization, error) {
	return db.finishNFCAuthorization(ctx, id, NFCStatusReversed)
}

func (db *DB) ExpireNFCHolds(ctx context.Context, limit int) ([]*NFCAuthorization, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("nfc: begin expire tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx, `
SELECT id, idempotency_key, token_id, wallet_address, network, merchant_id, terminal_id, COALESCE(external_ref,''),
       amount_brl_minor, COALESCE(fee_brl_minor,0), COALESCE(total_brl_minor, amount_brl_minor), COALESCE(fee_bps,0),
       usdt_rate::float8, required_usdt_micro, status, response_code, COALESCE(reason,''),
       hold_expires_at, created_at, updated_at
FROM nfc_authorizations
WHERE status = 'approved'
  AND hold_expires_at IS NOT NULL
  AND hold_expires_at <= NOW()
ORDER BY hold_expires_at
FOR UPDATE SKIP LOCKED
LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("nfc: select expired holds: %w", err)
	}
	defer rows.Close()

	var expired []*NFCAuthorization
	for rows.Next() {
		auth, err := scanNFCAuthorization(rows)
		if err != nil {
			return nil, err
		}
		res, err := tx.ExecContext(ctx, `
UPDATE nfc_wallet_balances
SET available_usdt_micro = available_usdt_micro + $3,
    locked_usdt_micro = locked_usdt_micro - $3,
    updated_at = NOW()
WHERE wallet_address = $1 AND network = $2 AND asset = 'USDT'
  AND locked_usdt_micro >= $3`,
			strings.ToLower(auth.Wallet), normalizeNFCNetwork(auth.Network), auth.RequiredUSDTMic)
		if err != nil {
			return nil, fmt.Errorf("nfc: expire balance %s: %w", auth.ID, err)
		}
		if affected, err := res.RowsAffected(); err != nil {
			return nil, err
		} else if affected != 1 {
			return nil, fmt.Errorf("nfc: authorization %s has no matching locked balance", auth.ID)
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE nfc_authorizations
SET status='expired', reason='hold_expired', expired_at=NOW(), updated_at=NOW()
WHERE id=$1 AND status='approved'`, auth.ID); err != nil {
			return nil, fmt.Errorf("nfc: expire authorization %s: %w", auth.ID, err)
		}
		auth.Status = NFCStatusExpired
		auth.Reason = "hold_expired"
		expired = append(expired, auth)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("nfc: commit expire holds: %w", err)
	}
	return expired, nil
}

func (db *DB) finishNFCAuthorization(ctx context.Context, id, finalStatus string) (*NFCAuthorization, error) {
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("nfc: begin finish tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	auth, err := txGetNFCAuthorizationByID(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if auth == nil {
		return nil, nil
	}
	if auth.Status == finalStatus {
		return auth, tx.Commit()
	}
	if auth.Status != NFCStatusApproved {
		return nil, fmt.Errorf("nfc: authorization %s is %s, not approved", id, auth.Status)
	}

	var balanceResult sql.Result
	switch finalStatus {
	case NFCStatusCaptured:
		balanceResult, err = tx.ExecContext(ctx, `
UPDATE nfc_wallet_balances
SET locked_usdt_micro = locked_usdt_micro - $3,
    updated_at = NOW()
WHERE wallet_address = $1 AND network = $2 AND asset = 'USDT'
  AND locked_usdt_micro >= $3`,
			strings.ToLower(auth.Wallet), normalizeNFCNetwork(auth.Network), auth.RequiredUSDTMic)
	case NFCStatusReversed:
		balanceResult, err = tx.ExecContext(ctx, `
UPDATE nfc_wallet_balances
SET available_usdt_micro = available_usdt_micro + $3,
    locked_usdt_micro = locked_usdt_micro - $3,
    updated_at = NOW()
WHERE wallet_address = $1 AND network = $2 AND asset = 'USDT'
  AND locked_usdt_micro >= $3`,
			strings.ToLower(auth.Wallet), normalizeNFCNetwork(auth.Network), auth.RequiredUSDTMic)
	default:
		return nil, fmt.Errorf("nfc: unsupported final status %s", finalStatus)
	}
	if err != nil {
		return nil, fmt.Errorf("nfc: update balance for %s: %w", finalStatus, err)
	}
	if rows, err := balanceResult.RowsAffected(); err != nil {
		return nil, fmt.Errorf("nfc: verify balance update for %s: %w", finalStatus, err)
	} else if rows != 1 {
		return nil, fmt.Errorf("nfc: authorization %s has no matching locked balance", id)
	}

	timestampColumn := "captured_at"
	if finalStatus == NFCStatusReversed {
		timestampColumn = "reversed_at"
	}
	q := fmt.Sprintf(`
UPDATE nfc_authorizations
SET status=$2, %s=NOW(), updated_at=NOW()
WHERE id=$1 AND status='approved'`, timestampColumn)
	authResult, err := tx.ExecContext(ctx, q, auth.ID, finalStatus)
	if err != nil {
		return nil, fmt.Errorf("nfc: mark %s: %w", finalStatus, err)
	}
	if rows, err := authResult.RowsAffected(); err != nil {
		return nil, fmt.Errorf("nfc: verify authorization update for %s: %w", finalStatus, err)
	} else if rows != 1 {
		return nil, fmt.Errorf("nfc: authorization %s changed before %s", id, finalStatus)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("nfc: commit %s: %w", finalStatus, err)
	}
	return db.GetNFCAuthorization(ctx, id)
}

func (db *DB) captureNFCAuthorization(ctx context.Context, id string) (*NFCCaptureResult, error) {
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("nfc: begin capture tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	auth, err := txGetNFCAuthorizationByID(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if auth == nil {
		return nil, nil
	}
	if auth.Status == NFCStatusCaptured {
		settlement, err := txGetMerchantSettlementByAuthorization(ctx, tx, auth.ID)
		if err != nil {
			return nil, err
		}
		if settlement == nil {
			settlement, err = txCreateMerchantSettlementForCapture(ctx, tx, auth)
			if err != nil {
				return nil, err
			}
		}
		return &NFCCaptureResult{Authorization: auth, Settlement: settlement}, tx.Commit()
	}
	if auth.Status != NFCStatusApproved {
		return nil, fmt.Errorf("nfc: authorization %s is %s, not approved", id, auth.Status)
	}

	balanceResult, err := tx.ExecContext(ctx, `
UPDATE nfc_wallet_balances
SET locked_usdt_micro = locked_usdt_micro - $3,
    updated_at = NOW()
WHERE wallet_address = $1 AND network = $2 AND asset = 'USDT'
  AND locked_usdt_micro >= $3`,
		strings.ToLower(auth.Wallet), normalizeNFCNetwork(auth.Network), auth.RequiredUSDTMic)
	if err != nil {
		return nil, fmt.Errorf("nfc: update balance for capture: %w", err)
	}
	if rows, err := balanceResult.RowsAffected(); err != nil {
		return nil, fmt.Errorf("nfc: verify balance update for capture: %w", err)
	} else if rows != 1 {
		return nil, fmt.Errorf("nfc: authorization %s has no matching locked balance", id)
	}

	authResult, err := tx.ExecContext(ctx, `
UPDATE nfc_authorizations
SET status='captured', captured_at=NOW(), updated_at=NOW()
WHERE id=$1 AND status='approved'`, auth.ID)
	if err != nil {
		return nil, fmt.Errorf("nfc: mark capture: %w", err)
	}
	if rows, err := authResult.RowsAffected(); err != nil {
		return nil, fmt.Errorf("nfc: verify authorization update for capture: %w", err)
	} else if rows != 1 {
		return nil, fmt.Errorf("nfc: authorization %s changed before capture", id)
	}

	settlement, err := txCreateMerchantSettlementForCapture(ctx, tx, auth)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("nfc: commit capture: %w", err)
	}
	captured, err := db.GetNFCAuthorization(ctx, id)
	if err != nil {
		return nil, err
	}
	return &NFCCaptureResult{Authorization: captured, Settlement: settlement}, nil
}

func txCreateMerchantSettlementForCapture(ctx context.Context, tx *sql.Tx, auth *NFCAuthorization) (*MerchantSettlement, error) {
	if auth == nil {
		return nil, fmt.Errorf("nfc settlement: authorization is nil")
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO nfc_merchants (id, display_name, status)
VALUES ($1,$1,'active')
ON CONFLICT (id) DO NOTHING`, auth.MerchantID); err != nil {
		return nil, fmt.Errorf("nfc settlement: ensure merchant: %w", err)
	}
	var pixKey, document sql.NullString
	if err := tx.QueryRowContext(ctx, `
SELECT settlement_pix_key, settlement_document
FROM nfc_merchants
WHERE id = $1
FOR UPDATE`, auth.MerchantID).Scan(&pixKey, &document); err != nil {
		return nil, fmt.Errorf("nfc settlement: merchant lookup: %w", err)
	}
	settlementID := "nfc_settle_" + NewAccessToken()[:24]
	idempotencyKey := settlementID
	const q = `
INSERT INTO merchant_settlements
  (id, merchant_id, terminal_id, authorization_id, capture_id, amount_brl_minor, fee_brl_minor,
   provider, rail, status, idempotency_key, target_pix_key, target_document)
VALUES ($1,$2,$3,$4,$5,$6,$7,'efi','pix_send','PENDING',$8,$9,$10)
ON CONFLICT (authorization_id) DO UPDATE SET updated_at = merchant_settlements.updated_at
RETURNING id, merchant_id, terminal_id, authorization_id, capture_id, amount_brl_minor, fee_brl_minor,
          provider, rail, status, COALESCE(provider_reference,''), COALESCE(provider_status,''), COALESCE(txid,''),
          idempotency_key, COALESCE(target_pix_key,''), COALESCE(target_document,''), retry_count, next_retry_at,
          COALESCE(error_message,''), submitted_at, confirmed_at, failed_at, created_at, updated_at`
	settlement, err := scanMerchantSettlement(tx.QueryRowContext(ctx, q,
		settlementID, auth.MerchantID, auth.TerminalID, auth.ID, auth.ID, auth.AmountBRLMinor, auth.FeeBRLMinor,
		idempotencyKey, pixKey, document,
	))
	if err != nil {
		return nil, fmt.Errorf("nfc settlement: create: %w", err)
	}
	return settlement, nil
}

func (db *DB) GetMerchantSettlement(ctx context.Context, id string) (*MerchantSettlement, error) {
	const q = `
SELECT id, merchant_id, terminal_id, authorization_id, capture_id, amount_brl_minor, fee_brl_minor,
       provider, rail, status, COALESCE(provider_reference,''), COALESCE(provider_status,''), COALESCE(txid,''),
       idempotency_key, COALESCE(target_pix_key,''), COALESCE(target_document,''), retry_count, next_retry_at,
       COALESCE(error_message,''), submitted_at, confirmed_at, failed_at, created_at, updated_at
FROM merchant_settlements
WHERE id = $1`
	settlement, err := scanMerchantSettlement(db.SQL.QueryRowContext(ctx, q, strings.TrimSpace(id)))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return settlement, err
}

func (db *DB) GetDueMerchantSettlements(ctx context.Context, limit int) ([]MerchantSettlement, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	const q = `
SELECT id, merchant_id, terminal_id, authorization_id, capture_id, amount_brl_minor, fee_brl_minor,
       provider, rail, status, COALESCE(provider_reference,''), COALESCE(provider_status,''), COALESCE(txid,''),
       idempotency_key, COALESCE(target_pix_key,''), COALESCE(target_document,''), retry_count, next_retry_at,
       COALESCE(error_message,''), submitted_at, confirmed_at, failed_at, created_at, updated_at
FROM merchant_settlements
WHERE status IN ('PENDING','SUBMITTED')
  AND next_retry_at <= NOW()
ORDER BY created_at
LIMIT $1`
	rows, err := db.SQL.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("nfc settlement: list due: %w", err)
	}
	defer rows.Close()
	var out []MerchantSettlement
	for rows.Next() {
		settlement, err := scanMerchantSettlement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *settlement)
	}
	return out, rows.Err()
}

func (db *DB) ClaimMerchantSettlement(ctx context.Context, id string) (*MerchantSettlement, bool, error) {
	const q = `
UPDATE merchant_settlements
SET status='SUBMITTED',
    retry_count=retry_count+1,
    submitted_at=COALESCE(submitted_at, NOW()),
    updated_at=NOW()
WHERE id = $1
  AND status IN ('PENDING','SUBMITTED')
  AND next_retry_at <= NOW()
RETURNING id, merchant_id, terminal_id, authorization_id, capture_id, amount_brl_minor, fee_brl_minor,
          provider, rail, status, COALESCE(provider_reference,''), COALESCE(provider_status,''), COALESCE(txid,''),
          idempotency_key, COALESCE(target_pix_key,''), COALESCE(target_document,''), retry_count, next_retry_at,
          COALESCE(error_message,''), submitted_at, confirmed_at, failed_at, created_at, updated_at`
	settlement, err := scanMerchantSettlement(db.SQL.QueryRowContext(ctx, q, strings.TrimSpace(id)))
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return settlement, true, nil
}

func (db *DB) MarkMerchantSettlementConfirmed(ctx context.Context, id, providerReference, providerStatus, txid string) error {
	_, err := db.SQL.ExecContext(ctx, `
UPDATE merchant_settlements
SET status='CONFIRMED',
    provider_reference=$2,
    provider_status=$3,
    txid=$4,
    error_message=NULL,
    confirmed_at=NOW(),
    updated_at=NOW()
WHERE id=$1 AND status IN ('PENDING','SUBMITTED')`,
		strings.TrimSpace(id), nullableString(strings.TrimSpace(providerReference)), nullableString(strings.TrimSpace(providerStatus)), nullableString(strings.TrimSpace(txid)))
	return err
}

func (db *DB) MarkMerchantSettlementFailed(ctx context.Context, id, errMsg string, permanent bool) error {
	status := MerchantSettlementStatusSubmitted
	if permanent {
		status = MerchantSettlementStatusFailed
	}
	_, err := db.SQL.ExecContext(ctx, `
UPDATE merchant_settlements
SET status=$2,
    error_message=$3,
    failed_at=CASE WHEN $2 = 'FAILED' THEN NOW() ELSE failed_at END,
    next_retry_at=CASE
      WHEN $2 = 'FAILED' THEN next_retry_at
      ELSE NOW() + (LEAST(60, POWER(2, GREATEST(retry_count, 1)))::INT * INTERVAL '1 minute')
    END,
    updated_at=NOW()
WHERE id=$1 AND status IN ('PENDING','SUBMITTED')`,
		strings.TrimSpace(id), status, strings.TrimSpace(errMsg))
	return err
}

func txGetMerchantSettlementByAuthorization(ctx context.Context, tx *sql.Tx, authorizationID string) (*MerchantSettlement, error) {
	const q = `
SELECT id, merchant_id, terminal_id, authorization_id, capture_id, amount_brl_minor, fee_brl_minor,
       provider, rail, status, COALESCE(provider_reference,''), COALESCE(provider_status,''), COALESCE(txid,''),
       idempotency_key, COALESCE(target_pix_key,''), COALESCE(target_document,''), retry_count, next_retry_at,
       COALESCE(error_message,''), submitted_at, confirmed_at, failed_at, created_at, updated_at
FROM merchant_settlements
WHERE authorization_id = $1`
	settlement, err := scanMerchantSettlement(tx.QueryRowContext(ctx, q, strings.TrimSpace(authorizationID)))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return settlement, err
}

func scanMerchantSettlement(row scanner) (*MerchantSettlement, error) {
	var s MerchantSettlement
	var submittedAt, confirmedAt, failedAt sql.NullTime
	err := row.Scan(
		&s.ID, &s.MerchantID, &s.TerminalID, &s.AuthorizationID, &s.CaptureID, &s.AmountBRLMinor, &s.FeeBRLMinor,
		&s.Provider, &s.Rail, &s.Status, &s.ProviderReference, &s.ProviderStatus, &s.TXID,
		&s.IdempotencyKey, &s.TargetPixKey, &s.TargetDocument, &s.RetryCount, &s.NextRetryAt,
		&s.ErrorMessage, &submittedAt, &confirmedAt, &failedAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if submittedAt.Valid {
		s.SubmittedAt = &submittedAt.Time
	}
	if confirmedAt.Valid {
		s.ConfirmedAt = &confirmedAt.Time
	}
	if failedAt.Valid {
		s.FailedAt = &failedAt.Time
	}
	return &s, nil
}

func txInsertNFCAuthorization(ctx context.Context, tx *sql.Tx, in NFCAuthorizeInput, status, responseCode, reason string, holdExpires any) (*NFCAuthorization, bool, error) {
	if in.ID == "" {
		in.ID = NewID()
	}
	const q = `
INSERT INTO nfc_authorizations
  (id, idempotency_key, token_id, token_hash, wallet_address, network, merchant_id, terminal_id, external_ref,
   amount_brl_minor, fee_brl_minor, total_brl_minor, fee_bps, usdt_rate, required_usdt_micro, status, response_code, reason, hold_expires_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
RETURNING id, idempotency_key, token_id, wallet_address, network, merchant_id, terminal_id, COALESCE(external_ref,''),
          amount_brl_minor, COALESCE(fee_brl_minor,0), COALESCE(total_brl_minor, amount_brl_minor), COALESCE(fee_bps,0),
          usdt_rate::float8, required_usdt_micro, status, response_code, COALESCE(reason,''),
          hold_expires_at, created_at, updated_at`
	auth, err := scanNFCAuthorization(tx.QueryRowContext(ctx, q,
		in.ID, strings.TrimSpace(in.IdempotencyKey), in.TokenID, in.TokenHash,
		strings.ToLower(strings.TrimSpace(in.Wallet)), normalizeNFCNetwork(in.Network),
		strings.TrimSpace(in.MerchantID), strings.TrimSpace(in.TerminalID), nullableString(strings.TrimSpace(in.ExternalRef)),
		in.AmountBRLMinor, in.FeeBRLMinor, firstNonZeroInt64(in.TotalBRLMinor, in.AmountBRLMinor), in.FeeBps,
		in.USDTRate, in.RequiredUSDTMic, status, responseCode, reason, holdExpires,
	))
	if err != nil {
		return nil, false, fmt.Errorf("nfc: insert authorization: %w", err)
	}
	return auth, false, tx.Commit()
}

func txGetNFCAuthorizationByIdempotency(ctx context.Context, tx *sql.Tx, terminalID, key string) (*NFCAuthorization, error) {
	const q = `
SELECT id, idempotency_key, token_id, wallet_address, network, merchant_id, terminal_id, COALESCE(external_ref,''),
       amount_brl_minor, COALESCE(fee_brl_minor,0), COALESCE(total_brl_minor, amount_brl_minor), COALESCE(fee_bps,0),
       usdt_rate::float8, required_usdt_micro, status, response_code, COALESCE(reason,''),
       hold_expires_at, created_at, updated_at
FROM nfc_authorizations
WHERE terminal_id = $1 AND idempotency_key = $2
FOR UPDATE`
	auth, err := scanNFCAuthorization(tx.QueryRowContext(ctx, q, strings.TrimSpace(terminalID), strings.TrimSpace(key)))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return auth, err
}

func sameNFCAuthorizationPayload(a *NFCAuthorization, in NFCAuthorizeInput) bool {
	if a == nil {
		return false
	}
	return strings.EqualFold(a.Wallet, in.Wallet) &&
		normalizeNFCNetwork(a.Network) == normalizeNFCNetwork(in.Network) &&
		strings.TrimSpace(a.MerchantID) == strings.TrimSpace(in.MerchantID) &&
		strings.TrimSpace(a.TerminalID) == strings.TrimSpace(in.TerminalID) &&
		strings.TrimSpace(a.ExternalRef) == strings.TrimSpace(in.ExternalRef) &&
		a.AmountBRLMinor == in.AmountBRLMinor &&
		a.FeeBRLMinor == in.FeeBRLMinor &&
		a.TotalBRLMinor == firstNonZeroInt64(in.TotalBRLMinor, in.AmountBRLMinor) &&
		a.FeeBps == in.FeeBps &&
		a.RequiredUSDTMic == in.RequiredUSDTMic
}

func txGetNFCAuthorizationByID(ctx context.Context, tx *sql.Tx, id string) (*NFCAuthorization, error) {
	const q = `
SELECT id, idempotency_key, token_id, wallet_address, network, merchant_id, terminal_id, COALESCE(external_ref,''),
       amount_brl_minor, COALESCE(fee_brl_minor,0), COALESCE(total_brl_minor, amount_brl_minor), COALESCE(fee_bps,0),
       usdt_rate::float8, required_usdt_micro, status, response_code, COALESCE(reason,''),
       hold_expires_at, created_at, updated_at
FROM nfc_authorizations
WHERE id = $1
FOR UPDATE`
	auth, err := scanNFCAuthorization(tx.QueryRowContext(ctx, q, strings.TrimSpace(id)))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return auth, err
}

func scanNFCAuthorization(row scanner) (*NFCAuthorization, error) {
	var a NFCAuthorization
	var hold sql.NullTime
	err := row.Scan(&a.ID, &a.IdempotencyKey, &a.TokenID, &a.Wallet, &a.Network, &a.MerchantID, &a.TerminalID, &a.ExternalRef,
		&a.AmountBRLMinor, &a.FeeBRLMinor, &a.TotalBRLMinor, &a.FeeBps,
		&a.USDTRate, &a.RequiredUSDTMic, &a.Status, &a.ResponseCode, &a.Reason, &hold, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if hold.Valid {
		a.HoldExpiresAt = &hold.Time
	}
	return &a, nil
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func scanNFCBalance(row scanner) (*NFCBalance, error) {
	var b NFCBalance
	err := row.Scan(&b.Wallet, &b.Network, &b.Asset, &b.AvailableMicro, &b.LockedMicro, &b.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func normalizeNFCNetwork(network string) string {
	network = strings.ToUpper(strings.TrimSpace(network))
	if network == "" {
		return "BSC"
	}
	return network
}
