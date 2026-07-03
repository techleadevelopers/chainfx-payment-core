package models

import (
	"time"
)

//OrderStatus define os estados possíveis de uma ordem no sistema
type OrderStatus string

const (
	StatusAguardandoDeposito OrderStatus = "aguardando_deposito"
	StatusAguardandoValidacao OrderStatus = "aguardando_validacao" // Caso fuja da tolerância de %
	StatusExpirada           OrderStatus = "expirada"
	StatusProcessandoPayout  OrderStatus = "processando_payout"
	StatusConcluida          OrderStatus = "concluida"
	StatusErro               OrderStatus = "erro"
)

// Order representa a tabela 'orders' do seu banco de dados Postgres
type Order struct {
	ID                string      `json:"id"`                  // UUID da ordem
	AmountBRL         float64     `json:"amount_brl"`          // Valor que o usuário quer receber em R$
	AmountUSDT        float64     `json:"amount_usdt"`         // Valor calculado em USDT que ele deve enviar
	Status            OrderStatus `json:"status"`              // Status atual (enum)
	PixKey            string      `json:"pix_key"`             // Chave PIX de destino (CPF ou Telefone)
	PixType           string      `json:"pix_type"`            // "cpf" ou "phone"
	TronAddress       string      `json:"tron_address"`        // Endereço gerado/derivado via XPUB para ele depositar
	TxHash            *string     `json:"tx_hash,omitempty"`   // Hash da transação quando detectada (pode ser nulo)
	RateLockExpiresAt time.Time   `json:"rate_lock_expires_at"` // TTL da cotação (Order expiration)
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

// OrderMeta representa os dados de auditoria que seu Node grava em order.meta
type OrderMeta struct {
	OrderID   string    `json:"order_id"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
}

// OnchainCursor ajuda o onchainWorker a saber de onde parou na paginação da TRON
type OnchainCursor struct {
	ID         int       `json:"id"`
	LastBlock  uint64    `json:"last_block"`
	UpdatedAt  time.Time `json:"updated_at"`
}