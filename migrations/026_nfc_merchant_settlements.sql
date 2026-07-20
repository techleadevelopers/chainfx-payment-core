-- Durable merchant settlement queue for ChainFX Tap captures.
-- Authorize and capture stay internal; this table drives asynchronous Efí Pix
-- Send payouts after the customer has already left the POS.

CREATE TABLE IF NOT EXISTS merchant_settlements (
  id TEXT PRIMARY KEY,
  merchant_id TEXT NOT NULL REFERENCES nfc_merchants(id),
  terminal_id TEXT NOT NULL,
  authorization_id TEXT NOT NULL UNIQUE REFERENCES nfc_authorizations(id),
  capture_id TEXT NOT NULL,
  amount_brl_minor BIGINT NOT NULL CHECK (amount_brl_minor > 0),
  fee_brl_minor BIGINT NOT NULL DEFAULT 0 CHECK (fee_brl_minor >= 0),
  provider TEXT NOT NULL DEFAULT 'efi',
  rail TEXT NOT NULL DEFAULT 'pix_send',
  status TEXT NOT NULL CHECK (status IN ('PENDING','SUBMITTED','CONFIRMED','FAILED')),
  provider_reference TEXT,
  provider_status TEXT,
  txid TEXT,
  idempotency_key TEXT NOT NULL UNIQUE,
  target_pix_key TEXT,
  target_document TEXT,
  retry_count INT NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  error_message TEXT,
  submitted_at TIMESTAMPTZ,
  confirmed_at TIMESTAMPTZ,
  failed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_merchant_settlements_status_retry
  ON merchant_settlements(status, next_retry_at, created_at)
  WHERE status IN ('PENDING','SUBMITTED');

CREATE INDEX IF NOT EXISTS idx_merchant_settlements_merchant_created
  ON merchant_settlements(merchant_id, created_at DESC);
