package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"payment-gateway/internal/config"
	"payment-gateway/internal/database"
	"payment-gateway/internal/workers"
)

func main() {
	log.Println("Iniciando o ecossistema concorrente em Go...")

	cfg := config.LoadConfig()

	db, err := database.ConnectPostgres(cfg)
	if err != nil {
		log.Fatalf("Erro fatal ao conectar no banco de dados: %v", err)
	}
	defer db.Close()

	// 1. Criamos um Contexto cancelável para gerenciar o desligamento ordenado da aplicação
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. Inicializa o gerenciador e dispara os Workers de Produção
	workerMgr := workers.NewWorkerManager(db)
	workerMgr.StartAll(ctx)

	log.Println("Todos os motores em background foram disparados e isolados.")

	// 3. Captura sinais de desligamento do terminal (Ctrl+C, SIGTERM do Docker/Kubernetes)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop // O código "trava" aqui de forma eficiente até receber um comando de parada
	log.Println("Sinal de encerramento recebido. Desligando sistemas de forma limpa...")

	// Cancela o contexto principal, avisando a todas as Goroutines de background para pararem imediatamente
	cancel()

	// Pequeno delay de cortesia para os workers fecharem conexões abertas com segurança
	time.Sleep(1 * time.Second)
	log.Println("Aplicação finalizada com 100% de segurança de dados.")
}