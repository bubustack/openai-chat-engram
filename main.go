package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	sdk "github.com/bubustack/bubu-sdk-go"
	chat "github.com/bubustack/openai-chat-engram/pkg/engram"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := sdk.Start(ctx, chat.New()); err != nil {
		log.Fatalf("openai-chat engram failed: %v", err)
	}
}
