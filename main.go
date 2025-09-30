package main

import (
	"context"

	chat "openai-chat/pkg/engram"

	sdk "github.com/bubustack/bubu-sdk-go"
)

func main() {
	// Run in batch mode by default. Operator sets execution mode.
	_ = sdk.Run(context.Background(), chat.New())
}
