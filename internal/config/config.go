package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config centraliza todas as variáveis do seu .env de forma tipada e segura
type Config struct {
	DatabaseURL             string
	AllowedOrigins          string
	WebhookSecret           string
	OrderMinBrl             float64
	OrderMaxBrl             float64
	
	// Tron / USDT TRC20
	TronFullNodeURL        string
	TronSolidityURL        string
	TronUsdtContract       string
	TronUsdtDecimals       int
	TronConfirmations       int
	TronXPub                string
	TronHmacSecret          string
	
	// Regras de Limite e Fraude
	PixMaxOrdersPer24h      int
	PixMaxBrlPer24h         float64
	OrderHoldSecForNewDest  int
	TronDepositTolerancePct float64
	
	// PagBank
	PagSeguroApiToken       string
	PagSeguroApiBaseUrl     string
	PixWebhookSecret        string
}

// LoadConfig é o cara que lê o .env e joga para dentro da estrutura acima
func LoadConfig() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("Aviso: Arquivo .env não encontrado, usando variáveis de ambiente do sistema")
	}

	return &Config{
		DatabaseURL:             getEnv("DATABASE_URL", ""),
		AllowedOrigins:          getEnv("ALLOWED_ORIGINS", "http://localhost:5173"),
		WebhookSecret:           getEnv("WEBHOOK_SECRET", ""),
		OrderMinBrl:             getEnvAsFloat("ORDER_MIN_BRL", 10.0),
		OrderMaxBrl:             getEnvAsFloat("ORDER_MAX_BRL", 10000.0),
		
		TronFullNodeURL:        getEnv("TRON_FULLNODE_URL", ""),
		TronSolidityURL:        getEnv("TRON_SOLIDITY_URL", ""),
		TronUsdtContract:       getEnv("TRON_USDT_CONTRACT", ""),
		TronUsdtDecimals:       getEnvAsInt("TRON_USDT_DECIMALS", 6),
		TronConfirmations:       getEnvAsInt("TRON_CONFIRMATIONS", 20),
		TronXPub:                getEnv("TRON_XPUB", ""),
		TronHmacSecret:          getEnv("TRON_HMAC_SECRET", ""),
		
		PixMaxOrdersPer24h:      getEnvAsInt("PIX_MAX_ORDERS_PER_24H", 5),
		PixMaxBrlPer24h:         getEnvAsFloat("PIX_MAX_BRL_PER_24H", 20000.0),
		OrderHoldSecForNewDest:  getEnvAsInt("ORDER_HOLD_SEC_FOR_NEW_DEST", 180),
		TronDepositTolerancePct: getEnvAsFloat("TRON_DEPOSIT_TOLERANCE_PCT", 0.02),
		
		PagSeguroApiToken:       getEnv("PAGSEGURO_API_TOKEN", ""),
		PagSeguroApiBaseUrl:     getEnv("PAGSEGURO_API_BASE_URL", "https://api.pagseguro.com"),
		PixWebhookSecret:        getEnv("PIX_WEBHOOK_SECRET", ""),
	}
}

// Auxiliares para leitura e conversão de tipos
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}

func getEnvAsFloat(key string, defaultValue float64) float64 {
	valueStr := getEnv(key, "")
	if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
		return value
	}
	return defaultValue
}