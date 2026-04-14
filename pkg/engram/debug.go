package engram

import (
	"context"
	"log/slog"

	sdk "github.com/bubustack/bubu-sdk-go"
)

func (e *ChatEngram) debugEnabled(ctx context.Context, logger *slog.Logger) bool {
	if sdk.DebugModeEnabled() {
		return true
	}
	if logger == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return logger.Enabled(ctx, slog.LevelDebug)
}

func logChatDebugPrompts(logger *slog.Logger, enabled bool, model string, input *Input) {
	if !enabled || logger == nil || input == nil {
		return
	}
	payload := map[string]any{
		"model":             model,
		"systemPrompt":      input.SystemPrompt,
		"developerPrompt":   input.DeveloperPrompt,
		"userPrompt":        input.UserPrompt,
		"assistantPrompt":   input.AssistantPrompt,
		"history":           input.History,
		"modalities":        input.Modalities,
		"responseFormat":    input.ResponseFormat,
		"tools":             input.Tools,
		"toolChoice":        input.ToolChoice,
		"allowedTools":      input.AllowedTools,
		"webSearch":         input.WebSearch,
		"prediction":        input.Prediction,
		"metadata":          input.Metadata,
		"logitBias":         input.LogitBias,
		"parallelToolCalls": input.ParallelToolCalls,
		"logprobs":          input.Logprobs,
		"topLogprobs":       input.TopLogprobs,
		"choices":           input.Choices,
	}
	logger.Debug("openai chat prompts", slog.Any("payload", payload))
}

func logChatDebugOutput(logger *slog.Logger, enabled bool, output *Output) {
	if !enabled || logger == nil || output == nil {
		return
	}
	logger.Debug("openai chat output",
		slog.String("model", output.Model),
		slog.String("provider", output.Provider),
		slog.String("text", output.Text),
		slog.Any("structured", output.Structured),
		slog.Any("actionRequest", output.ActionRequest),
		slog.Any("toolCalls", output.ToolCalls),
	)
}

func truncateText(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "…"
}
