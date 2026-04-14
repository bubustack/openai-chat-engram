package engram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	cfg "github.com/bubustack/openai-chat-engram/pkg/config"
	"github.com/openai/openai-go/v2/packages/param"
	"github.com/openai/openai-go/v2/responses"
	"github.com/openai/openai-go/v2/shared"
	"github.com/openai/openai-go/v2/shared/constant"
)

func (e *ChatEngram) runResponses(
	ctx context.Context,
	input *Input,
	model string,
	logger *slog.Logger,
) (*Output, error) {
	params, err := e.buildResponseParams(input, model)
	if err != nil {
		return nil, err
	}
	logger.Debug("dispatching OpenAI responses request", "model", model)
	resp, err := e.client.Responses.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai responses request failed: %w", err)
	}
	output := e.mapResponsesToOutput(resp)
	output.Model = model
	if output.Provider == "" {
		output.Provider = e.providerName()
	}
	logChatDebugOutput(logger, e.debugEnabled(ctx, logger), output)
	if text := strings.TrimSpace(output.Text); text != "" {
		logger.Info("OpenAI responses output",
			"model", output.Model,
			"textPreview", truncateText(text, 160),
			"toolCalls", len(output.ToolCalls),
		)
	}
	return output, nil
}

func (e *ChatEngram) buildResponseParams(input *Input, model string) (responses.ResponseNewParams, error) {
	var params responses.ResponseNewParams
	params.Model = model
	messages := e.buildResponseInputItems(input)
	if len(messages) == 0 {
		return params, fmt.Errorf("prompt content is required")
	}
	params.Input = responses.ResponseNewParamsInputUnion{
		OfInputItemList: messages,
	}

	e.applyResponseSampling(&params, input)
	e.applyResponseMetadata(&params, input)
	e.applyResponseTextConfig(&params, input)
	e.applyResponseTools(&params, input)

	return params, nil
}

func (e *ChatEngram) applyResponseSampling(params *responses.ResponseNewParams, input *Input) {
	if temp := e.resolveTemperature(input); temp >= 0 {
		params.Temperature = param.NewOpt(float64(temp))
	}
	if topP := firstFloat(input.TopP, e.config.DefaultTopP); topP > 0 {
		params.TopP = param.NewOpt(float64(topP))
	}
	if maxTokens := firstInt(input.MaxTokens, e.config.DefaultMaxTokens); maxTokens > 0 {
		params.MaxOutputTokens = param.NewOpt(int64(maxTokens))
	}
	if maxCompletion := firstInt(input.MaxCompletion, e.config.DefaultMaxCompletion); maxCompletion > 0 {
		params.MaxOutputTokens = param.NewOpt(int64(maxCompletion))
	}
	if store := firstBool(input.Store, e.config.DefaultStore); store != nil {
		params.Store = param.NewOpt(*store)
	}
	if parallel := firstBool(input.ParallelToolCalls, e.config.DefaultParallelToolCalls); parallel != nil {
		params.ParallelToolCalls = param.NewOpt(*parallel)
	}
	if topLog := firstInt(input.TopLogprobs, e.config.DefaultTopLogprobs); topLog > 0 {
		params.TopLogprobs = param.NewOpt(int64(topLog))
	}
}

func (e *ChatEngram) applyResponseMetadata(params *responses.ResponseNewParams, input *Input) {
	if merged := mergeStringMap(e.config.DefaultMetadata, input.Metadata); len(merged) > 0 {
		params.Metadata = shared.Metadata(merged)
	}
	if promptCache := firstString(input.PromptCacheKey, e.config.DefaultPromptCacheKey); promptCache != "" {
		params.PromptCacheKey = param.NewOpt(promptCache)
	}
	if safetyID := firstString(input.SafetyIdentifier, e.config.DefaultSafetyIdentifier); safetyID != "" {
		params.SafetyIdentifier = param.NewOpt(safetyID)
	}
	if user := firstString(input.User, e.config.DefaultUser); user != "" {
		params.User = param.NewOpt(user)
	}
	if serviceTier := firstString(input.ServiceTier, e.config.DefaultServiceTier); serviceTier != "" {
		params.ServiceTier = responses.ResponseNewParamsServiceTier(strings.ToLower(serviceTier))
	}
}

func (e *ChatEngram) applyResponseTextConfig(params *responses.ResponseNewParams, input *Input) {
	if verbosity := firstString(input.Verbosity, e.config.DefaultVerbosity); verbosity != "" {
		ensureResponseTextConfig(params)
		params.Text.Verbosity = responses.ResponseTextConfigVerbosity(strings.ToLower(verbosity))
	}
	spec := input.ResponseFormat
	if spec == nil {
		spec = e.config.ResponseFormat
	}
	if formatUnion := buildResponsesFormat(spec); formatUnion != nil {
		ensureResponseTextConfig(params)
		params.Text.Format = *formatUnion
	}
}

func ensureResponseTextConfig(params *responses.ResponseNewParams) {
	if params.Text == (responses.ResponseTextConfigParam{}) {
		params.Text = responses.ResponseTextConfigParam{}
	}
}

func (e *ChatEngram) applyResponseTools(
	params *responses.ResponseNewParams,
	input *Input,
) {
	choice := resolveToolChoice(input, e.config)
	e.applyResponsesToolChoice(params, choice)
	allowed := e.config.AllowedTools
	if input.AllowedTools != nil {
		allowed = cfg.NormalizeAllowedTools(input.AllowedTools)
	}
	tools := mergeToolSpecs(e.config.Tools, e.config.Functions)
	if len(input.Tools) > 0 || len(input.Functions) > 0 {
		tools = mergeToolSpecs(input.Tools, input.Functions)
	}
	responseTools, toolRegistry := buildResponseTools(tools)
	if len(responseTools) > 0 {
		params.Tools = responseTools
	}
	e.applyResponsesAllowedTools(params, allowed, toolRegistry)
}

func (e *ChatEngram) buildResponseInputItems(input *Input) responses.ResponseInputParam {
	items := make(responses.ResponseInputParam, 0, len(input.History)+4)
	add := func(role responses.EasyInputMessageRole, content string) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		msgContent := responses.ResponseInputMessageContentListParam{
			responses.ResponseInputContentParamOfInputText(content),
		}
		items = append(items, responses.ResponseInputItemParamOfMessage(msgContent, role))
	}
	systemPrompt := strings.TrimSpace(e.config.DefaultSystemPrompt)
	if input.SystemPrompt != "" {
		systemPrompt = strings.TrimSpace(input.SystemPrompt)
	}
	if systemPrompt != "" {
		add(responses.EasyInputMessageRoleSystem, systemPrompt)
	}
	if dev := strings.TrimSpace(input.DeveloperPrompt); dev != "" {
		add(responses.EasyInputMessageRoleDeveloper, dev)
	}
	for _, msg := range input.History {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		var mapped responses.EasyInputMessageRole
		switch role {
		case "system":
			mapped = responses.EasyInputMessageRoleSystem
		case "assistant":
			mapped = responses.EasyInputMessageRoleAssistant
		case "developer":
			mapped = responses.EasyInputMessageRoleDeveloper
		default:
			mapped = responses.EasyInputMessageRoleUser
		}
		add(mapped, msg.Content)
	}
	if assistant := strings.TrimSpace(input.AssistantPrompt); assistant != "" {
		add(responses.EasyInputMessageRoleAssistant, assistant)
	}
	if user := strings.TrimSpace(resolvePromptString(input.UserPrompt)); user != "" {
		add(responses.EasyInputMessageRoleUser, user)
	}
	return items
}

func buildResponseTools(specs []cfg.ToolSpec) ([]responses.ToolUnionParam, map[string]struct{}) {
	if len(specs) == 0 {
		return nil, nil
	}
	tools := make([]responses.ToolUnionParam, 0, len(specs))
	registry := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			continue
		}
		fn := responses.FunctionToolParam{
			Name:       name,
			Parameters: spec.Parameters,
			Type:       constant.Function("function"),
		}
		fn.Strict = param.NewOpt(true)
		if desc := strings.TrimSpace(spec.Description); desc != "" {
			fn.Description = param.NewOpt(desc)
		}
		tools = append(tools, responses.ToolUnionParam{OfFunction: &fn})
		registry[name] = struct{}{}
	}
	if len(tools) == 0 {
		return nil, nil
	}
	return tools, registry
}

func buildResponsesFormat(spec *cfg.ResponseFormat) *responses.ResponseFormatTextConfigUnionParam {
	if spec == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(spec.Type)) {
	case "":
		return nil
	case "text":
		format := shared.ResponseFormatTextParam{Type: constant.Text("text")}
		return &responses.ResponseFormatTextConfigUnionParam{OfText: &format}
	case "json_object":
		format := shared.ResponseFormatJSONObjectParam{Type: constant.JSONObject("json_object")}
		return &responses.ResponseFormatTextConfigUnionParam{OfJSONObject: &format}
	case "json_schema":
		if spec.JSONSchema == nil || strings.TrimSpace(spec.JSONSchema.Name) == "" {
			return nil
		}
		jsonSchema := responses.ResponseFormatTextJSONSchemaConfigParam{
			Name:   spec.JSONSchema.Name,
			Schema: spec.JSONSchema.Schema,
			Type:   constant.JSONSchema("json_schema"),
		}
		if desc := strings.TrimSpace(spec.JSONSchema.Description); desc != "" {
			jsonSchema.Description = param.NewOpt(desc)
		}
		if spec.JSONSchema.Strict != nil {
			jsonSchema.Strict = param.NewOpt(*spec.JSONSchema.Strict)
		}
		return &responses.ResponseFormatTextConfigUnionParam{OfJSONSchema: &jsonSchema}
	default:
		return nil
	}
}

func (e *ChatEngram) applyResponsesToolChoice(params *responses.ResponseNewParams, choice string) {
	choice = strings.TrimSpace(strings.ToLower(choice))
	switch choice {
	case "", toolChoiceAuto:
		return
	case toolChoiceRequired:
		params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsRequired),
		}
	case toolChoiceNone:
		params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsNone),
		}
	default:
		if strings.HasPrefix(choice, "function:") {
			name := strings.TrimSpace(choice[len("function:"):])
			if name != "" {
				params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
					OfFunctionTool: &responses.ToolChoiceFunctionParam{
						Name: name,
						Type: constant.Function("function"),
					},
				}
			}
		}
	}
}

func (e *ChatEngram) applyResponsesAllowedTools(
	params *responses.ResponseNewParams,
	allowed *cfg.AllowedToolsConfig,
	registry map[string]struct{},
) {
	if allowed == nil || len(allowed.Tools) == 0 {
		return
	}
	entries := buildAllowedResponseTools(allowed, registry)
	if len(entries) == 0 {
		return
	}
	mode := strings.ToLower(strings.TrimSpace(allowed.Mode))
	respMode := responses.ToolChoiceAllowedModeAuto
	if mode == toolChoiceRequired {
		respMode = responses.ToolChoiceAllowedModeRequired
	}
	params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
		OfAllowedTools: &responses.ToolChoiceAllowedParam{
			Mode:  respMode,
			Tools: entries,
		},
	}
}

func buildAllowedResponseTools(allowed *cfg.AllowedToolsConfig, registry map[string]struct{}) []map[string]any {
	if allowed == nil || len(allowed.Tools) == 0 {
		return nil
	}
	items := make([]map[string]any, 0, len(allowed.Tools))
	for _, name := range allowed.Tools {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if len(registry) > 0 {
			if _, ok := registry[name]; !ok {
				continue
			}
		}
		items = append(items, map[string]any{
			"type": "function",
			"name": name,
		})
	}
	return items
}

func (e *ChatEngram) mapResponsesToOutput(resp *responses.Response) *Output {
	if resp == nil {
		return &Output{Provider: e.providerName()}
	}
	out := &Output{
		Model:    resp.Model,
		Provider: e.providerName(),
	}
	var textBuilder strings.Builder
	var toolCalls []ToolCall
	for _, item := range resp.Output {
		switch actual := item.AsAny().(type) {
		case responses.ResponseOutputMessage:
			for _, content := range actual.Content {
				switch body := content.AsAny().(type) {
				case responses.ResponseOutputText:
					textBuilder.WriteString(body.Text)
				case responses.ResponseOutputRefusal:
					textBuilder.WriteString(body.Refusal)
				}
			}
		case responses.ResponseFunctionToolCall:
			toolCalls = append(toolCalls, ToolCall{
				Name:      actual.Name,
				Arguments: actual.Arguments,
			})
		}
	}
	out.Text = textBuilder.String()
	if len(toolCalls) > 0 {
		out.ToolCalls = toolCalls
		e.enrichToolOutputs(out)
	}
	return out
}
