package engram

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	sdkengram "github.com/bubustack/bubu-sdk-go/engram"
	openai "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/azure"
	"github.com/openai/openai-go/v2/option"

	cfg "openai-chat/pkg/config"
)

type ChatEngram struct {
	client      *openai.Client
	config      *cfg.Config
	isAzure     bool
	azureDep    string
	azureAPIVer string
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Input struct {
	History         []ChatMessage `json:"history"`
	Temperature     *float32      `json:"temperature,omitempty"`
	Model           string        `json:"model,omitempty"`
	SystemPrompt    string        `json:"systemPrompt,omitempty"`
	UserPrompt      string        `json:"userPrompt,omitempty"`
	DeveloperPrompt string        `json:"developerPrompt,omitempty"`
	AssistantPrompt string        `json:"assistantPrompt,omitempty"`
}

type ActionRequest struct {
	StoryName string      `json:"storyName"`
	Inputs    interface{} `json:"inputs"`
}

type ToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Output struct {
	Text          string         `json:"text"`
	Structured    interface{}    `json:"structured,omitempty"`
	ActionRequest *ActionRequest `json:"actionRequest,omitempty"`
	ToolCalls     []ToolCall     `json:"toolCalls,omitempty"`
}

func New() *ChatEngram { return &ChatEngram{} }

func (e *ChatEngram) Init(ctx context.Context, cfgIn cfg.Config, _ *sdkengram.Secrets) error {
	e.config = &cfgIn
	// Azure
	if ep := os.Getenv("AZURE_ENDPOINT"); ep != "" {
		key := os.Getenv("AZURE_API_KEY")
		if key == "" {
			return fmt.Errorf("AZURE_API_KEY is required when AZURE_ENDPOINT is set")
		}
		apiVersion := os.Getenv("AZURE_API_VERSION")
		if apiVersion == "" {
			apiVersion = "2024-06-01"
		}
		opts := []option.RequestOption{azure.WithEndpoint(ep, apiVersion), azure.WithAPIKey(key)}
		c := openai.NewClient(opts...)
		e.client = &c
		e.isAzure = true
		e.azureAPIVer = apiVersion
		e.azureDep = os.Getenv("AZURE_DEPLOYMENT")
		return nil
	}
	// OpenAI
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required")
	}
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if base := os.Getenv("OPENAI_BASE_URL"); base != "" {
		opts = append(opts, option.WithBaseURL(strings.TrimRight(base, "/")))
	}
	if org := os.Getenv("OPENAI_ORG_ID"); org != "" {
		opts = append(opts, option.WithOrganization(org))
	}
	c := openai.NewClient(opts...)
	e.client = &c
	return nil
}

func (e *ChatEngram) Process(ctx context.Context, execCtx *sdkengram.ExecutionContext, in Input) (*sdkengram.Result, error) {
	req := e.buildChatRequest(&in)
	resp, err := e.createChatCompletion(ctx, req)
	if err != nil {
		return &sdkengram.Result{Error: err}, nil
	}
	out := e.mapResponseToOutput(resp)
	return &sdkengram.Result{Data: out}, nil
}

func (e *ChatEngram) Stream(ctx context.Context, in <-chan []byte, out chan<- []byte) error {
	for data := range in {
		var input Input
		if err := json.Unmarshal(data, &input); err != nil {
			continue
		}
		req := e.buildChatRequest(&input)
		resp, err := e.createChatCompletion(ctx, req)
		if err != nil {
			continue
		}
		bytes, err := json.Marshal(e.mapResponseToOutput(resp))
		if err != nil {
			continue
		}
		select {
		case out <- bytes:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return ctx.Err()
}

func (e *ChatEngram) buildChatRequest(input *Input) map[string]interface{} {
	messages := make([]map[string]interface{}, 0, len(input.History)+3)
	systemPrompt := e.config.DefaultSystemPrompt
	if input.SystemPrompt != "" {
		systemPrompt = input.SystemPrompt
	}
	if systemPrompt != "" {
		messages = append(messages, map[string]interface{}{"role": "system", "content": systemPrompt})
	}
	if input.DeveloperPrompt != "" {
		messages = append(messages, map[string]interface{}{"role": "system", "content": input.DeveloperPrompt})
	}
	for _, m := range input.History {
		role := m.Role
		if role == "developer" {
			role = "system"
		}
		messages = append(messages, map[string]interface{}{"role": role, "content": m.Content})
	}
	if input.AssistantPrompt != "" {
		messages = append(messages, map[string]interface{}{"role": "assistant", "content": input.AssistantPrompt})
	}
	if input.UserPrompt != "" {
		messages = append(messages, map[string]interface{}{"role": "user", "content": input.UserPrompt})
	}

	temp := e.config.DefaultTemperature
	if input.Temperature != nil {
		temp = *input.Temperature
	}
	model := e.config.DefaultModel
	if input.Model != "" {
		model = input.Model
	}

	payload := map[string]interface{}{"model": model, "temperature": temp, "messages": messages}
	if len(e.config.Tools) > 0 {
		tools := make([]map[string]interface{}, 0, len(e.config.Tools))
		for _, t := range e.config.Tools {
			tools = append(tools, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": t.Name, "description": t.Description, "parameters": t.Parameters,
				},
			})
		}
		payload["tools"] = tools
		switch e.config.ToolChoice {
		case "required":
			payload["tool_choice"] = "required"
		case "none":
			payload["tool_choice"] = "none"
		}
	}
	return payload
}

func (e *ChatEngram) createChatCompletion(ctx context.Context, payload map[string]interface{}) (*chatAPIResponse, error) {
	var out chatAPIResponse
	if e.isAzure {
		dep := e.azureDep
		if dep == "" {
			if m, _ := payload["model"].(string); m != "" {
				dep = m
			}
		}
		if dep == "" {
			return nil, fmt.Errorf("azure deployment name is required (AZURE_DEPLOYMENT or inputs.model)")
		}
		path := fmt.Sprintf("/deployments/%s/chat/completions?api-version=%s", dep, e.azureAPIVer)
		delete(payload, "model")
		if err := e.client.Post(ctx, path, payload, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
	if err := e.client.Post(ctx, "/chat/completions", payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type chatAPIResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

func (e *ChatEngram) mapResponseToOutput(resp *chatAPIResponse) *Output {
	if len(resp.Choices) == 0 {
		return &Output{Text: ""}
	}
	choice := resp.Choices[0]
	out := &Output{Text: choice.Message.Content}
	if len(choice.Message.ToolCalls) > 0 {
		out.ToolCalls = make([]ToolCall, 0, len(choice.Message.ToolCalls))
		for _, call := range choice.Message.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, ToolCall{Name: call.Function.Name, Arguments: call.Function.Arguments})
			if e.config != nil && e.config.StructuredToolName == call.Function.Name {
				var structured interface{}
				if err := json.Unmarshal([]byte(call.Function.Arguments), &structured); err == nil {
					out.Structured = structured
				}
			}
			if e.config != nil && e.config.DispatchTools != nil {
				if storyName, ok := e.config.DispatchTools[call.Function.Name]; ok {
					var maybeAR ActionRequest
					if err := json.Unmarshal([]byte(call.Function.Arguments), &maybeAR); err == nil && maybeAR.StoryName != "" {
						out.ActionRequest = &maybeAR
					} else {
						var args interface{}
						_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
						out.ActionRequest = &ActionRequest{StoryName: storyName, Inputs: args}
					}
				}
			}
		}
	}
	return out
}
