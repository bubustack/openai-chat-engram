package engram

import (
	"context"
	"testing"
	"time"

	sdkengram "github.com/bubustack/bubu-sdk-go/engram"
	cfg "github.com/bubustack/openai-chat-engram/pkg/config"
	openai "github.com/openai/openai-go/v2"
)

func TestResolveSecretNilSafe(t *testing.T) {
	if got := resolveSecret(nil, "OPENAI_API_KEY"); got != "" {
		t.Fatalf("expected empty value for nil secrets, got %q", got)
	}
}

func TestResolveToolChoiceSupportsLegacyFunctionCallAlias(t *testing.T) {
	cfgIn := &cfg.Config{ToolChoice: "auto", FunctionCall: "function:dispatch_story"}
	in := &Input{FunctionCall: "none"}
	if got := resolveToolChoice(in, cfgIn); got != "none" {
		t.Fatalf("expected input functionCall alias to win, got %q", got)
	}
	in = &Input{}
	if got := resolveToolChoice(in, cfgIn); got != "auto" {
		t.Fatalf("expected config toolChoice to win, got %q", got)
	}
}

func TestMergeToolSpecsMergesLegacyFunctionsWithoutDuplicates(t *testing.T) {
	tools := []cfg.ToolSpec{
		{Name: "search"},
	}
	functions := []cfg.ToolSpec{
		{Name: "search"},
		{Name: "dispatch"},
	}
	merged := mergeToolSpecs(tools, functions)
	if len(merged) != 2 {
		t.Fatalf("expected two merged tool specs, got %d", len(merged))
	}
	if merged[0].Name != "search" || merged[1].Name != "dispatch" {
		t.Fatalf("unexpected merge order: %#v", merged)
	}
}

func TestMapResponseToOutputAggregatesMultipleChoices(t *testing.T) {
	engine := &ChatEngram{}
	resp := &openai.ChatCompletion{
		Model: "gpt-4o-mini",
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Content: "first"}},
			{Message: openai.ChatCompletionMessage{Content: "second"}},
		},
	}
	out := engine.mapResponseToOutput(resp)
	if out.Text != "first\n\nsecond" {
		t.Fatalf("expected aggregated text, got %q", out.Text)
	}
}

func TestStreamContinuesAfterMalformedPacket(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	engine := &ChatEngram{}
	in := make(chan sdkengram.InboundMessage, 1)
	out := make(chan sdkengram.StreamMessage, 1)

	in <- sdkengram.NewInboundMessage(sdkengram.StreamMessage{
		Payload: []byte("{invalid json"),
	})
	close(in)

	if err := engine.Stream(ctx, in, out); err != nil {
		t.Fatalf("expected malformed packet to be skipped, got err=%v", err)
	}
}
