package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/bubustack/bubu-sdk-go/engram"
	sdkruntime "github.com/bubustack/bubu-sdk-go/runtime"
	"github.com/sashabaranov/go-openai"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Input struct {
	Text    string        `json:"text"`
	History []ChatMessage `json:"history"`
}

type Output struct {
	Text          string         `json:"text"`
	ActionRequest *ActionRequest `json:"actionRequest,omitempty"`
}

type ActionRequest struct {
	StoryName string      `json:"storyName"`
	Inputs    interface{} `json:"inputs"`
}

type OpenAIChatEngram struct {
	client *openai.Client
	config struct {
		Model        string  `json:"model"`
		Temperature  float32 `json:"temperature"`
		SystemPrompt string  `json:"systemPrompt"`
	}
}

func (e *OpenAIChatEngram) Init(ctx context.Context, config *engram.Config, secrets *engram.Secrets) error {
	if err := config.Unmarshal(&e.config); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	apiKey, err := secrets.Get("openai", "API_KEY")
	if err != nil {
		return fmt.Errorf("failed to get openai api key: %w", err)
	}
	e.client = openai.NewClient(apiKey)
	return nil
}

// Process handles a single, non-streaming request to the OpenAI API.
func (e *OpenAIChatEngram) Process(ctx context.Context, execCtx *engram.ExecutionContext) (*engram.Result, error) {
	logger := execCtx.Logger()
	logger.Info("OpenAI Chat batch processing started")

	var input Input
	// Wrap the hydrated inputs in a Config object to use the Unmarshal helper.
	if err := engram.NewConfig(execCtx.Inputs()).Unmarshal(&input); err != nil {
		return &engram.Result{Error: fmt.Errorf("failed to unmarshal inputs: %w", err)}, nil
	}

	messages := e.buildMessages(&input)

	req := openai.ChatCompletionRequest{
		Model:       e.config.Model,
		Temperature: e.config.Temperature,
		Messages:    messages,
	}

	resp, err := e.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return &engram.Result{Error: fmt.Errorf("openai api call failed: %w", err)}, nil
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		output := &Output{
			Text: choice.Message.Content,
		}
		return &engram.Result{Data: output}, nil
	}

	return &engram.Result{Data: &Output{Text: ""}}, nil // Return empty output if no choices
}

func (e *OpenAIChatEngram) HandleStream(ctx context.Context, execCtx *engram.ExecutionContext, stream engram.Stream[Input, Output]) error {
	logger := execCtx.Logger()
	logger.Info("OpenAI Chat stream started, waiting for input...")

	for {
		input, err := stream.Recv(ctx)
		if err != nil {
			if err == io.EOF {
				logger.Info("Input stream closed.")
				return nil
			}
			return fmt.Errorf("failed to receive input: %w", err)
		}

		messages := e.buildMessages(input)

		req := openai.ChatCompletionRequest{
			Model:       e.config.Model,
			Temperature: e.config.Temperature,
			Messages:    messages,
		}

		resp, err := e.client.CreateChatCompletion(ctx, req)
		if err != nil {
			logger.Error("OpenAI API call failed", "error", err)
			continue
		}

		if len(resp.Choices) > 0 {
			choice := resp.Choices[0]
			output := &Output{
				Text: choice.Message.Content,
			}

			// Check if the model decided to call a tool
			if len(choice.Message.ToolCalls) > 0 {
				toolCall := choice.Message.ToolCalls[0]
				if toolCall.Function.Name == "run_story" {
					logger.Info("Model requested to run a story", "args", toolCall.Function.Arguments)
					// Parse the arguments and create an ActionRequest
					var actionReq ActionRequest
					if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &actionReq); err == nil {
						output.ActionRequest = &actionReq
					} else {
						logger.Error("Failed to unmarshal tool arguments", "error", err)
					}
				}
			}

			if err := stream.Send(ctx, output); err != nil {
				return fmt.Errorf("failed to send output: %w", err)
			}
		}
	}
}

func (e *OpenAIChatEngram) buildMessages(input *Input) []openai.ChatCompletionMessage {
	messages := make([]openai.ChatCompletionMessage, 0, len(input.History)+2)

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: e.config.SystemPrompt,
	})

	for _, msg := range input.History {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: input.Text,
	})

	return messages
}

func main() {
	engramToRun := &OpenAIChatEngram{}
	runtime, err := sdkruntime.New(engramToRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create sdk runtime: %v\n", err)
		os.Exit(1)
	}
	if err := runtime.Execute(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "engram execution failed: %v\n", err)
		os.Exit(1)
	}
}
