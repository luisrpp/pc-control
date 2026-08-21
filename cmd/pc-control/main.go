package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/luisrpp/pc-control/internal/server"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	service, err := server.NewFromEnv()
	if err != nil {
		log.Printf("pc-control startup failed: %v", err)
		return 1
	}

	log.Print("pc-control starting")
	if err := service.Run(ctx); err != nil {
		log.Printf("pc-control runtime failure: %v", err)
		return 1
	}

	log.Print("pc-control stopped")
	return 0
}
