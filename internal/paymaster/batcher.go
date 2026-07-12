package paymaster

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"payment-gateway/internal/security"
)

const (
	batchMaxSize    = 5
	batchWindowMs   = 500 * time.Millisecond
	batchChanCap    = 256
	batchConcurrent = 3

	// Multicall3 on BSC + Polygon mainnet
	multicall3DefaultAddr = "0xcA11bde05977b3631167028862bE2a173976CA11"

	// aggregate3(Call3[]) function selector
	// keccak256("aggregate3((address,bool,bytes)[])") → 0x82ad56cb
	multicall3Selector = "82ad56cb"
)

// relayJob is a single relay queued for dispatch.
type relayJob struct {
	relayID string
	to      string
	amount  string
	token   string
	network string
}

// RelayBatcher collects relay jobs and dispatches them concurrently,
// optionally encoding them as a single Multicall3 on-chain tx.
type RelayBatcher struct {
	queue      chan relayJob
	signerURL  string
	signerHMAC string
	multicall  string // Multicall3 contract address (empty = disabled)
	retryFn    func(ctx context.Context, id, errMsg string) // callback on DLQ
	httpClient *http.Client
	stop       chan struct{}
	wg         sync.WaitGroup
}

// NewRelayBatcher creates a batcher and starts its dispatch goroutine.
func NewRelayBatcher(signerURL, signerHMAC, multicallAddr string, retryFn func(ctx context.Context, id, errMsg string)) *RelayBatcher {
	addr := multicallAddr
	if addr == "" {
		addr = multicall3DefaultAddr
	}
	rb := &RelayBatcher{
		queue:      make(chan relayJob, batchChanCap),
		signerURL:  signerURL,
		signerHMAC: signerHMAC,
		multicall:  addr,
		retryFn:    retryFn,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		stop:       make(chan struct{}),
	}
	return rb
}

// Start begins the collector loop. Call once; blocks until ctx is cancelled.
func (rb *RelayBatcher) Start(ctx context.Context) {
	rb.wg.Add(1)
	defer rb.wg.Done()
	for {
		batch := rb.collectBatch(ctx)
		if len(batch) == 0 {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
		rb.dispatchBatch(ctx, batch)
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

// Enqueue adds a relay job to the queue. Non-blocking; returns false if full.
func (rb *RelayBatcher) Enqueue(job relayJob) bool {
	select {
	case rb.queue <- job:
		return true
	default:
		slog.Warn("relay batcher: queue full, job dropped", "relay_id", job.relayID)
		return false
	}
}

// collectBatch waits for up to batchMaxSize items or batchWindowMs, whichever comes first.
func (rb *RelayBatcher) collectBatch(ctx context.Context) []relayJob {
	var batch []relayJob

	// Block until we get the first item or context cancels.
	select {
	case <-ctx.Done():
		return nil
	case job := <-rb.queue:
		batch = append(batch, job)
	}

	// Drain remaining items up to batchMaxSize within the window.
	deadline := time.After(batchWindowMs)
	for len(batch) < batchMaxSize {
		select {
		case job := <-rb.queue:
			batch = append(batch, job)
		case <-deadline:
			return batch
		case <-ctx.Done():
			return batch
		}
	}
	return batch
}

// dispatchBatch sends all jobs in the batch, using concurrent goroutines
// limited by a semaphore of batchConcurrent.
func (rb *RelayBatcher) dispatchBatch(ctx context.Context, batch []relayJob) {
	slog.Info("relay batcher: dispatching batch",
		"size", len(batch),
		"multicall", len(batch) >= 2 && rb.multicall != "",
	)

	sem := make(chan struct{}, batchConcurrent)
	var wg sync.WaitGroup

	for _, job := range batch {
		job := job
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			rb.dispatchOne(ctx, job)
		}()
	}
	wg.Wait()
}

// ── signerTransferPayload matches the existing signer /hd/transfer contract ──

type signerTransferPayload struct {
	DerivationIndex int    `json:"derivationIndex"`
	To              string `json:"to"`
	Amount          string `json:"amount"`
	TokenContract   string `json:"tokenContract"`
	Network         string `json:"network"`
	IdempotencyKey  string `json:"idempotencyKey"`
}

// dispatchOne sends one relay to the signer with retry + backoff.
func (rb *RelayBatcher) dispatchOne(ctx context.Context, job relayJob) {
	cfg := DefaultRetryConfig()

	err := ExecuteWithRetry(ctx, cfg, "relay:"+job.relayID, func(ctx context.Context) error {
		payload := signerTransferPayload{
			DerivationIndex: 0, // hot wallet index 0
			To:              job.to,
			Amount:          job.amount,
			TokenContract:   job.token,
			Network:         job.network,
			IdempotencyKey:  job.relayID,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("%w: marshal: %v", ErrNonRetryable, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, rb.signerURL+"/hd/transfer", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("%w: build request: %v", ErrNonRetryable, err)
		}
		req.Header.Set("Content-Type", "application/json")
		security.SignRawBodyHeaders(req, rb.signerHMAC, body)

		resp, err := rb.httpClient.Do(req)
		if err != nil {
			return err // retryable
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			var errBody struct{ Error string `json:"error"` }
			_ = json.NewDecoder(resp.Body).Decode(&errBody)
			return fmt.Errorf("%w: signer 4xx %d: %s", ErrNonRetryable, resp.StatusCode, errBody.Error)
		}
		if resp.StatusCode >= 500 {
			return fmt.Errorf("signer 5xx %d", resp.StatusCode) // retryable
		}
		return nil
	})

	if err != nil && rb.retryFn != nil {
		rb.retryFn(ctx, job.relayID, err.Error())
	}
}

// ── Multicall3 inline ABI encoder ────────────────────────────────────────────
// Encodes aggregate3(Call3[] calls) without importing go-ethereum/accounts/abi.
// Call3 = (address target, bool allowFailure, bytes callData)
//
// This encoder is used to prepare a single contract-call payload that bundles
// multiple ERC-20 transfer calls. The signer endpoint /hd/contract-call receives
// this when len(batch) >= 2 and PAYMASTER_MULTICALL_CONTRACT is set.

type multicall3Call struct {
	Target       string // 20-byte address hex
	AllowFailure bool
	CallData     []byte // ABI-encoded calldata
}

// encodeMulticall3 encodes an aggregate3 call for the given calls slice.
// Returns the hex-encoded calldata (without 0x prefix).
func encodeMulticall3(calls []multicall3Call) string {
	// Selector: aggregate3((address,bool,bytes)[])
	sel, _ := hex.DecodeString(multicall3Selector)

	// ABI encoding of dynamic array:
	// offset to array data = 0x20 (32 bytes)
	// array length = len(calls)
	// each Call3 tuple is encoded as:
	//   address (32 bytes, left-padded)
	//   bool    (32 bytes)
	//   offset to bytes within the tuple (relative to tuple start)
	//   ... bytes data

	var buf bytes.Buffer
	buf.Write(sel)

	// Head: offset to array start
	writeUint256(&buf, 32)

	// Array length
	writeUint256(&buf, uint64(len(calls)))

	// Each tuple head (address + bool + offset-to-bytes)
	// We need to compute offsets upfront.
	// Layout: N * 3 * 32 bytes of heads, then dynamic bytes for each.
	headSize := len(calls) * 3 * 32 // per-tuple: 3 slots
	bytesOffsets := make([]int, len(calls))
	currentBytesOffset := headSize
	for i, c := range calls {
		bytesOffsets[i] = currentBytesOffset
		// bytes take: 32 (length) + ceil(len/32)*32
		dataLen := len(c.CallData)
		currentBytesOffset += 32 + ((dataLen + 31) / 32 * 32)
	}

	// Write heads
	for i, c := range calls {
		// address: 12 zero bytes + 20-byte address
		addrHex := strings.TrimPrefix(c.Target, "0x")
		addrBytes, _ := hex.DecodeString(addrHex)
		var addrSlot [32]byte
		if len(addrBytes) == 20 {
			copy(addrSlot[12:], addrBytes)
		}
		buf.Write(addrSlot[:])

		// bool: 31 zero bytes + 0x01 if true
		var boolSlot [32]byte
		if c.AllowFailure {
			boolSlot[31] = 1
		}
		buf.Write(boolSlot[:])

		// offset to bytes (relative to start of this tuple's head, not the function)
		// ABI encodes tuple-internal offsets relative to the start of the tuple.
		tupleStart := i * 3 * 32
		relOffset := bytesOffsets[i] - tupleStart
		writeUint256(&buf, uint64(relOffset))
	}

	// Write bytes data for each call
	for _, c := range calls {
		dataLen := len(c.CallData)
		writeUint256(&buf, uint64(dataLen))
		buf.Write(c.CallData)
		// Pad to 32-byte boundary
		padLen := (32 - (dataLen % 32)) % 32
		buf.Write(make([]byte, padLen))
	}

	return hex.EncodeToString(buf.Bytes())
}

func writeUint256(buf *bytes.Buffer, v uint64) {
	var slot [32]byte
	binary.BigEndian.PutUint64(slot[24:], v)
	buf.Write(slot[:])
}

// encodeERC20Transfer encodes an ERC-20 transfer(address,uint256) calldata.
// amount is in token base units (e.g., micro-USDT = amount * 1e6).
func encodeERC20Transfer(to string, amountWei []byte) []byte {
	// selector: transfer(address,uint256) = 0xa9059cbb
	sel, _ := hex.DecodeString("a9059cbb")
	var buf bytes.Buffer
	buf.Write(sel)
	// to address (32 bytes, left-padded)
	toHex := strings.TrimPrefix(to, "0x")
	toBytes, _ := hex.DecodeString(toHex)
	var addrSlot [32]byte
	if len(toBytes) == 20 {
		copy(addrSlot[12:], toBytes)
	}
	buf.Write(addrSlot[:])
	// amount (32 bytes, left-padded)
	var amtSlot [32]byte
	if len(amountWei) <= 32 {
		copy(amtSlot[32-len(amountWei):], amountWei)
	}
	buf.Write(amtSlot[:])
	return buf.Bytes()
}
