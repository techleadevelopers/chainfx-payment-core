package database

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"meu-gateway-go/internal/config"
)

// DB guarda a referência para o pool de conexões do Postgres
type DB struct {
	Pool *pgxpool.Pool
}

// ConnectPostgres inicializa e testa a conexão com o banco de dados
func ConnectPostgres(cfg *config.Config) (*DB, error) {
	// 1. Cria o contexto de tempo limite para a tentativa de conexão (5 segundos)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Println("Conectando ao banco de dados PostgreSQL...")

	// 2. Cria o pool de conexões baseado na string do seu .env
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	// 3. Dá um Ping real no banco para garantir que a credencial e a rede estão funcionando
	if err := pool.Ping(ctx); err != nil {
		pool.Close() // Fecha o pool se falhar no ping
		return nil, err
	}

	log.Println("Conexão com PostgreSQL estabelecida com sucesso!")
	
	return &DB{Pool: pool}, nil
}

// Close encerra todas as conexões abertas do pool (usado quando o app desliga)
func (db *DB) Close() {
	if db.Pool != nil {
		db.Pool.Close()
	}
}