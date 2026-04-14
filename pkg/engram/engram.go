package engram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	sdk "github.com/bubustack/bubu-sdk-go"
	sdkcel "github.com/bubustack/bubu-sdk-go/cel"
	sdkengram "github.com/bubustack/bubu-sdk-go/engram"
	cfg "github.com/bubustack/openai-chat-engram/pkg/config"
	openai "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/azure"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/packages/param"
	"github.com/openai/openai-go/v2/shared"
	"github.com/openai/openai-go/v2/shared/constant"
)

const (
	componentName      = "openai-chat-engram"
	toolChoiceAuto     = "auto"
	toolChoiceRequired = "required"
	toolChoiceNone     = "none"
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
	History           []ChatMessage           `json:"history"`
	Choices           *int                    `json:"choices,omitempty"`
	Temperature       *float32                `json:"temperature,omitempty"`
	Model             string                  `json:"model,omitempty"`
	SystemPrompt      string                  `json:"systemPrompt,omitempty"`
	UserPrompt        any                     `json:"userPrompt,omitempty"`
	DeveloperPrompt   string                  `json:"developerPrompt,omitempty"`
	AssistantPrompt   string                  `json:"assistantPrompt,omitempty"`
	TopP              *float32                `json:"topP,omitempty"`
	MaxTokens         *int                    `json:"maxTokens,omitempty"`
	MaxCompletion     *int                    `json:"maxCompletionTokens,omitempty"`
	PresencePenalty   *float32                `json:"presencePenalty,omitempty"`
	FrequencyPenalty  *float32                `json:"frequencyPenalty,omitempty"`
	Stop              []string                `json:"stop,omitempty"`
	Modalities        []string                `json:"modalities,omitempty"`
	Store             *bool                   `json:"store,omitempty"`
	ParallelToolCalls *bool                   `json:"parallelToolCalls,omitempty"`
	Logprobs          *bool                   `json:"logprobs,omitempty"`
	TopLogprobs       *int                    `json:"topLogprobs,omitempty"`
	Seed              *int64                  `json:"seed,omitempty"`
	ServiceTier       string                  `json:"serviceTier,omitempty"`
	ReasoningEffort   string                  `json:"reasoningEffort,omitempty"`
	Verbosity         string                  `json:"verbosity,omitempty"`
	ResponseFormat    *cfg.ResponseFormat     `json:"responseFormat,omitempty"`
	Metadata          map[string]string       `json:"metadata,omitempty"`
	LogitBias         map[string]int64        `json:"logitBias,omitempty"`
	Audio             *cfg.AudioConfig        `json:"audio,omitempty"`
	PromptCacheKey    string                  `json:"promptCacheKey,omitempty"`
	SafetyIdentifier  string                  `json:"safetyIdentifier,omitempty"`
	User              string                  `json:"user,omitempty"`
	Tools             []cfg.ToolSpec          `json:"tools,omitempty"`
	Functions         []cfg.ToolSpec          `json:"functions,omitempty"`
	ToolChoice        string                  `json:"toolChoice,omitempty"`
	FunctionCall      string                  `json:"functionCall,omitempty"`
	Prediction        *PredictionInput        `json:"prediction,omitempty"`
	WebSearch         *cfg.WebSearchConfig    `json:"webSearch,omitempty"`
	AllowedTools      *cfg.AllowedToolsConfig `json:"allowedTools,omitempty"`
	UseResponsesAPI   *bool                   `json:"useResponsesAPI,omitempty"`
}

type PredictionInput struct {
	Text string `json:"text"`
}

type ActionRequest struct {
	StoryName string `json:"storyName"`
	Inputs    any    `json:"inputs"`
}

type ToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Output struct {
	Text          string         `json:"text"`
	Structured    any            `json:"structured,omitempty"`
	ActionRequest *ActionRequest `json:"actionRequest,omitempty"`
	ToolCalls     []ToolCall     `json:"toolCalls,omitempty"`
	Model         string         `json:"model,omitempty"`
	Provider      string         `json:"provider,omitempty"`
}

func New() *ChatEngram { return &ChatEngram{} }

func (e *ChatEngram) Init(ctx context.Context, cfgIn cfg.Config, secrets *sdkengram.Secrets) error {
	normalized := cfg.Normalize(cfgIn)
	e.config = &normalized

	// Use the SDK's secrets object for all secret access.
	// This decouples the engram from environment variables and makes it more secure and testable.
	if azureEndpoint := resolveSecret(secrets, "AZURE_ENDPOINT"); azureEndpoint != "" {
		apiKey := resolveSecret(secrets, "AZURE_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("secret 'AZURE_API_KEY' is required when 'AZURE_ENDPOINT' is set")
		}
		apiVersion := resolveSecret(secrets, "AZURE_API_VERSION")
		if apiVersion == "" {
			apiVersion = "2024-06-01"
		}
		opts := []option.RequestOption{azure.WithEndpoint(azureEndpoint, apiVersion), azure.WithAPIKey(apiKey)}
		c := openai.NewClient(opts...)
		e.client = &c
		e.isAzure = true
		e.azureAPIVer = apiVersion
		e.azureDep = resolveSecret(secrets, "AZURE_DEPLOYMENT")
		return nil
	}

	// Default to standard OpenAI client
	apiKey := resolveSecret(secrets, "OPENAI_API_KEY", "API_KEY")
	if apiKey == "" {
		return fmt.Errorf("secret 'OPENAI_API_KEY' is required")
	}
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL := resolveSecret(secrets, "OPENAI_BASE_URL", "BASE_URL"); baseURL != "" {
		opts = append(opts, option.WithBaseURL(strings.TrimRight(baseURL, "/")))
	}
	if orgID := resolveSecret(secrets, "OPENAI_ORG_ID", "ORG_ID"); orgID != "" {
		opts = append(opts, option.WithOrganization(orgID))
	}
	c := openai.NewClient(opts...)
	e.client = &c
	return nil
}

func (e *ChatEngram) Process(
	ctx context.Context,
	execCtx *sdkengram.ExecutionContext,
	in Input,
) (*sdkengram.Result, error) {
	logger := execCtx.Logger().With(
		"component", componentName,
		"mode", "batch",
	)
	e.resolveTemplatePrompts(ctx, execCtx, &in, logger)
	output, err := e.runChat(ctx, &in, logger)
	if err != nil {
		return nil, err
	}
	return sdkengram.NewResultFrom(output), nil
}

func (e *ChatEngram) Stream(
	ctx context.Context,
	in <-chan sdkengram.InboundMessage,
	out chan<- sdkengram.StreamMessage,
) error {
	baseLogger := sdk.LoggerFromContext(ctx).With(
		"component", componentName,
		"mode", "stream",
	)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-in:
			if !ok {
				return nil
			}
			if err := e.handleStreamMessage(ctx, msg, baseLogger, out); err != nil {
				return err
			}
		}
	}
}

func (e *ChatEngram) handleStreamMessage(
	ctx context.Context,
	msg sdkengram.InboundMessage,
	baseLogger *slog.Logger,
	out chan<- sdkengram.StreamMessage,
) error {
	msgLogger := baseLogger
	if msg.MessageID != "" {
		msgLogger = msgLogger.With("messageID", msg.MessageID)
	}
	input, ok := decodeStreamInput(msg, msgLogger, e.debugEnabled(ctx, msgLogger))
	if !ok {
		msg.Done()
		return nil
	}
	output, err := e.runChat(ctx, &input, msgLogger)
	if err != nil {
		msgLogger.Warn("Failed to process stream chat packet", "error", err)
		msg.Done()
		return nil
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		msgLogger.Warn("Failed to marshal stream chat response", "error", err)
		msg.Done()
		return nil
	}
	metadata := buildStreamMetadata(msg.Metadata, e.providerName(), output.Model)
	select {
	case out <- sdkengram.StreamMessage{
		Metadata: metadata,
		Inputs:   encoded,
		Payload:  encoded,
	}:
		msg.Done()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func decodeStreamInput(msg sdkengram.InboundMessage, logger *slog.Logger, debug bool) (Input, bool) {
	raw := streamInputBytes(msg)
	if len(raw) == 0 {
		if isHeartbeat(msg.Metadata) {
			logger.Debug("Ignoring heartbeat packet")
			return Input{}, false
		}
		logger.Warn("Stream message missing data (inputs, payload, and binary)", "metadata", msg.Metadata)
		return Input{}, false
	}
	var input Input
	if err := json.Unmarshal(raw, &input); err != nil {
		logger.Warn("Failed to decode streaming payload", "error", err)
		return Input{}, false
	}
	if shouldSkipStreamingInput(&input) {
		if debug {
			logger.Info("Skipping chat: empty prompt in streaming input")
		}
		return Input{}, false
	}
	return input, true
}

func buildStreamMetadata(source map[string]string, provider, model string) map[string]string {
	metadata := cloneMetadata(source)
	metadata["provider"] = provider
	if model != "" {
		metadata["model"] = model
	}
	metadata["type"] = "openai.chat.v1"
	return metadata
}

func streamInputBytes(msg sdkengram.InboundMessage) []byte {
	if len(msg.Inputs) > 0 {
		return msg.Inputs
	}
	if len(msg.Payload) > 0 {
		return msg.Payload
	}
	if msg.Binary != nil && len(msg.Binary.Payload) > 0 {
		return msg.Binary.Payload
	}
	return nil
}

func (e *ChatEngram) runChat(ctx context.Context, input *Input, logger *slog.Logger) (*Output, error) {
	model := e.resolveModel(input)
	logger.Info("OpenAI assistant request",
		"model", model,
		"promptPreview", truncateText(firstNonEmptyPrompt(input), 160),
		"historyMessages", len(input.History),
	)
	logChatDebugPrompts(logger, e.debugEnabled(ctx, logger), model, input)
	useResponses := e.shouldUseResponsesAPI(input)
	if useResponses {
		if e.isAzure {
			logger.Warn("Responses API is not available for Azure OpenAI; falling back to Chat Completions")
		} else {
			return e.runResponses(ctx, input, model, logger)
		}
	}
	req, err := e.buildChatRequest(input, logger)
	if err != nil {
		return nil, err
	}
	resp, err := e.createChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create chat completion: %w", err)
	}
	output := e.mapResponseToOutput(resp)
	if output.Model == "" {
		output.Model = model
	}
	output.Provider = e.providerName()
	logChatDebugOutput(logger, e.debugEnabled(ctx, logger), output)
	if text := strings.TrimSpace(output.Text); text != "" {
		logger.Info("OpenAI assistant response",
			"model", output.Model,
			"textPreview", truncateText(text, 160),
			"toolCalls", len(output.ToolCalls),
		)
	}
	emitChatSignal(ctx, logger, output)
	return output, nil
}

func (e *ChatEngram) resolveTemplatePrompts(
	ctx context.Context,
	execCtx *sdkengram.ExecutionContext,
	input *Input,
	logger *slog.Logger,
) {
	if execCtx == nil || input == nil {
		return
	}
	vars := execCtx.CELContext()
	if len(vars) == 0 {
		return
	}
	evaluator, err := sdkcel.NewEvaluator(logger, sdkcel.Config{})
	if err != nil {
		logger.Warn("Failed to initialize template evaluator for prompts", "error", err)
		return
	}
	defer evaluator.Close()

	resolveString := func(value string) string {
		if strings.TrimSpace(value) == "" {
			return value
		}
		resolved, err := evaluator.ResolveTemplate(ctx, value, vars)
		if err != nil {
			logger.Warn("Failed to resolve template string", "error", err)
			return value
		}
		if str, ok := resolved.(string); ok {
			return str
		}
		return fmt.Sprint(resolved)
	}

	input.SystemPrompt = resolveString(input.SystemPrompt)
	input.DeveloperPrompt = resolveString(input.DeveloperPrompt)
	input.AssistantPrompt = resolveString(input.AssistantPrompt)

	if input.UserPrompt != nil {
		resolved, err := evaluator.ResolveTemplate(ctx, input.UserPrompt, vars)
		if err != nil {
			logger.Warn("Failed to resolve template userPrompt", "error", err)
		} else {
			input.UserPrompt = resolved
		}
	}
}

func (e *ChatEngram) buildChatRequest(input *Input, logger *slog.Logger) (openai.ChatCompletionNewParams, error) {
	messages := e.composeMessages(input)
	model := e.resolveModel(input)
	params := openai.ChatCompletionNewParams{
		Model:    model,
		Messages: messages,
	}
	if choices := firstInt(input.Choices, e.config.DefaultChoices); choices > 1 {
		params.N = openai.Int(int64(choices))
	}
	if temp := e.resolveTemperature(input); temp >= 0 {
		params.Temperature = openai.Float(float64(temp))
	}
	e.applySamplingOptions(&params, input)
	if err := e.applyResponseFormat(&params, input); err != nil {
		return params, err
	}
	if err := e.applyAudioOptions(&params, input); err != nil {
		return params, err
	}
	e.applyTools(&params, input, logger)
	e.applyPrediction(&params, input)
	e.applyWebSearch(&params, input)
	e.applyMetadataOptions(&params, input)
	return params, nil
}

func (e *ChatEngram) composeMessages(input *Input) []openai.ChatCompletionMessageParamUnion {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(input.History)+4)
	e.addSystemPrompts(&messages, input)
	e.addHistoryMessages(&messages, input.History)
	e.addRuntimePrompts(&messages, input)
	return messages
}

func (e *ChatEngram) addSystemPrompts(
	messages *[]openai.ChatCompletionMessageParamUnion,
	input *Input,
) {
	systemPrompt := e.config.DefaultSystemPrompt
	if input.SystemPrompt != "" {
		systemPrompt = input.SystemPrompt
	}
	if systemPrompt != "" {
		*messages = append(*messages, openai.SystemMessage(systemPrompt))
	}
	if input.DeveloperPrompt != "" {
		*messages = append(*messages, openai.SystemMessage(input.DeveloperPrompt))
	}
}

func (e *ChatEngram) addHistoryMessages(
	messages *[]openai.ChatCompletionMessageParamUnion,
	history []ChatMessage,
) {
	for _, m := range history {
		switch strings.ToLower(m.Role) {
		case "system", "developer":
			*messages = append(*messages, openai.SystemMessage(m.Content))
		case "assistant":
			*messages = append(*messages, openai.AssistantMessage(m.Content))
		default:
			*messages = append(*messages, openai.UserMessage(m.Content))
		}
	}
}

func (e *ChatEngram) addRuntimePrompts(
	messages *[]openai.ChatCompletionMessageParamUnion,
	input *Input,
) {
	if input.AssistantPrompt != "" {
		*messages = append(*messages, openai.AssistantMessage(input.AssistantPrompt))
	}
	if prompt := resolvePromptString(input.UserPrompt); prompt != "" {
		*messages = append(*messages, openai.UserMessage(prompt))
	}
}

func resolvePromptString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.RawMessage:
		if len(v) == 0 {
			return ""
		}
		var decoded any
		if err := json.Unmarshal(v, &decoded); err == nil {
			return resolvePromptString(decoded)
		}
		return string(v)
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(encoded)
	}
}

func shouldSkipStreamingInput(input *Input) bool {
	if input == nil {
		return true
	}
	if len(input.History) > 0 {
		return false
	}
	if strings.TrimSpace(input.AssistantPrompt) != "" {
		return false
	}
	if strings.TrimSpace(input.DeveloperPrompt) != "" {
		return false
	}
	if strings.TrimSpace(resolvePromptString(input.UserPrompt)) != "" {
		return false
	}
	return true
}

func (e *ChatEngram) applySamplingOptions(params *openai.ChatCompletionNewParams, input *Input) {
	e.applyFloatSamplingOptions(params, input)
	e.applyTokenSamplingOptions(params, input)
	e.applyBehaviorSamplingOptions(params, input)
}

func (e *ChatEngram) applyFloatSamplingOptions(
	params *openai.ChatCompletionNewParams,
	input *Input,
) {
	if topP := firstFloat(input.TopP, e.config.DefaultTopP); topP > 0 {
		params.TopP = openai.Float(float64(topP))
	}
	if presence := firstFloat(input.PresencePenalty, e.config.DefaultPresencePenalty); presence != 0 {
		params.PresencePenalty = openai.Float(float64(presence))
	}
	if frequency := firstFloat(input.FrequencyPenalty, e.config.DefaultFrequencyPenalty); frequency != 0 {
		params.FrequencyPenalty = openai.Float(float64(frequency))
	}
}

func (e *ChatEngram) applyTokenSamplingOptions(
	params *openai.ChatCompletionNewParams,
	input *Input,
) {
	if maxTokens := firstInt(input.MaxTokens, e.config.DefaultMaxTokens); maxTokens > 0 {
		params.MaxTokens = openai.Int(int64(maxTokens))
	}
	if maxCompletion := firstInt(input.MaxCompletion, e.config.DefaultMaxCompletion); maxCompletion > 0 {
		params.MaxCompletionTokens = openai.Int(int64(maxCompletion))
	}
	if stop := resolveStopSequences(input.Stop, e.config.DefaultStopSequences); len(stop) == 1 {
		params.Stop = openai.ChatCompletionNewParamsStopUnion{OfString: openai.String(stop[0])}
	} else if len(stop) > 1 {
		params.Stop = openai.ChatCompletionNewParamsStopUnion{OfStringArray: stop}
	}
	if modalities := resolveModalities(input.Modalities, e.config.DefaultModalities); len(modalities) > 0 {
		params.Modalities = modalities
	}
}

func (e *ChatEngram) applyBehaviorSamplingOptions(
	params *openai.ChatCompletionNewParams,
	input *Input,
) {
	if store := firstBool(input.Store, e.config.DefaultStore); store != nil {
		params.Store = openai.Bool(*store)
	}
	if parallel := firstBool(input.ParallelToolCalls, e.config.DefaultParallelToolCalls); parallel != nil {
		params.ParallelToolCalls = openai.Bool(*parallel)
	}
	if logprobs := firstBool(input.Logprobs, e.config.DefaultLogprobs); logprobs != nil {
		params.Logprobs = openai.Bool(*logprobs)
	}
	if topLog := firstInt(input.TopLogprobs, e.config.DefaultTopLogprobs); topLog > 0 {
		params.TopLogprobs = openai.Int(int64(topLog))
	}
	if seed := firstInt64(input.Seed, e.config.DefaultSeed); seed != nil && *seed != 0 {
		params.Seed = openai.Int(*seed)
	}
	if tier := firstString(input.ServiceTier, e.config.DefaultServiceTier); tier != "" {
		params.ServiceTier = openai.ChatCompletionNewParamsServiceTier(strings.ToLower(tier))
	}
	if effort := firstString(input.ReasoningEffort, e.config.DefaultReasoningEffort); effort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(strings.ToLower(effort))
	}
	if verbosity := firstString(input.Verbosity, e.config.DefaultVerbosity); verbosity != "" {
		params.Verbosity = openai.ChatCompletionNewParamsVerbosity(strings.ToLower(verbosity))
	}
}

func (e *ChatEngram) applyResponseFormat(params *openai.ChatCompletionNewParams, input *Input) error {
	spec := input.ResponseFormat
	if spec == nil {
		spec = e.config.ResponseFormat
	}
	union, err := buildResponseFormatUnion(spec)
	if err != nil {
		return err
	}
	if union != nil {
		params.ResponseFormat = *union
	}
	return nil
}

func (e *ChatEngram) applyAudioOptions(params *openai.ChatCompletionNewParams, input *Input) error {
	config := input.Audio
	if config == nil {
		config = e.config.Audio
	}
	if config == nil {
		return nil
	}
	format, err := toAudioFormat(config.Format)
	if err != nil {
		return err
	}
	voice, err := toAudioVoice(config.Voice)
	if err != nil {
		return err
	}
	params.Audio = openai.ChatCompletionAudioParam{
		Format: format,
		Voice:  voice,
	}
	if len(params.Modalities) == 0 {
		params.Modalities = []string{"audio"}
	} else if !containsString(params.Modalities, "audio") {
		params.Modalities = append(params.Modalities, "audio")
	}
	return nil
}

func (e *ChatEngram) applyTools(
	params *openai.ChatCompletionNewParams,
	input *Input,
	logger *slog.Logger,
) {
	toolSpecs := mergeToolSpecs(e.config.Tools, e.config.Functions)
	if len(input.Tools) > 0 || len(input.Functions) > 0 {
		toolSpecs = mergeToolSpecs(input.Tools, input.Functions)
	}
	if len(toolSpecs) == 0 {
		return
	}
	tools, registry := e.buildToolParams(toolSpecs)
	params.Tools = tools
	choice := resolveToolChoice(input, e.config)
	e.applyToolChoice(params, choice)
	allowed := e.config.AllowedTools
	if input.AllowedTools != nil {
		allowed = cfg.NormalizeAllowedTools(input.AllowedTools)
	}
	e.applyAllowedTools(params, allowed, registry, logger)
}

func (e *ChatEngram) applyMetadataOptions(params *openai.ChatCompletionNewParams, input *Input) {
	if metadata := mergeStringMap(e.config.DefaultMetadata, input.Metadata); len(metadata) > 0 {
		params.Metadata = shared.Metadata(metadata)
	}
	if bias := resolveLogitBias(e.config.DefaultLogitBias, input.LogitBias); len(bias) > 0 {
		params.LogitBias = bias
	}
	if key := firstString(input.PromptCacheKey, e.config.DefaultPromptCacheKey); key != "" {
		params.PromptCacheKey = openai.String(key)
	}
	if id := firstString(input.SafetyIdentifier, e.config.DefaultSafetyIdentifier); id != "" {
		params.SafetyIdentifier = openai.String(id)
	}
	if user := firstString(input.User, e.config.DefaultUser); user != "" {
		params.User = openai.String(user)
	}
}

func isHeartbeat(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	value, ok := metadata["bubu-heartbeat"]
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

func emitChatSignal(ctx context.Context, logger *slog.Logger, output *Output) {
	if output == nil {
		return
	}
	opts := sdk.TextSignalOptions{
		Format:      "markdown",
		ContentType: "text/markdown",
		IncludeHash: true,
		Attributes: map[string]string{
			"model":    output.Model,
			"provider": output.Provider,
		},
		SampleExtras: func() map[string]any {
			if len(output.ToolCalls) == 0 {
				return nil
			}
			return map[string]any{
				"toolCalls": output.ToolCalls,
			}
		}(),
	}
	if err := sdk.EmitTextSignal(ctx, "chat.response", output.Text, opts); err != nil &&
		!errors.Is(err, sdk.ErrSignalsUnavailable) {
		logger.Warn("Failed to emit chat response signal", "error", err)
	}
}

func (e *ChatEngram) applyPrediction(params *openai.ChatCompletionNewParams, input *Input) {
	if input.Prediction == nil {
		return
	}
	text := strings.TrimSpace(input.Prediction.Text)
	if text == "" {
		return
	}
	params.Prediction = openai.ChatCompletionPredictionContentParam{
		Type: constant.ValueOf[constant.Content](),
		Content: openai.ChatCompletionPredictionContentContentUnionParam{
			OfString: param.NewOpt(text),
		},
	}
}

func (e *ChatEngram) applyWebSearch(params *openai.ChatCompletionNewParams, input *Input) {
	search := e.config.WebSearch
	if input.WebSearch != nil {
		search = cfg.NormalizeWebSearch(input.WebSearch)
	}
	if search == nil {
		return
	}
	options := openai.ChatCompletionNewParamsWebSearchOptions{}
	hasLocation := false
	if s := strings.TrimSpace(strings.ToLower(search.SearchContextSize)); s != "" {
		options.SearchContextSize = s
	}
	if search.UserLocation != nil {
		loc := openai.ChatCompletionNewParamsWebSearchOptionsUserLocation{
			Type: constant.ValueOf[constant.Approximate](),
		}
		var approx openai.ChatCompletionNewParamsWebSearchOptionsUserLocationApproximate
		if city := strings.TrimSpace(search.UserLocation.City); city != "" {
			approx.City = param.NewOpt(city)
			hasLocation = true
		}
		if country := strings.TrimSpace(search.UserLocation.Country); country != "" {
			approx.Country = param.NewOpt(strings.ToUpper(country))
			hasLocation = true
		}
		if region := strings.TrimSpace(search.UserLocation.Region); region != "" {
			approx.Region = param.NewOpt(region)
			hasLocation = true
		}
		if tz := strings.TrimSpace(search.UserLocation.Timezone); tz != "" {
			approx.Timezone = param.NewOpt(tz)
			hasLocation = true
		}
		if hasLocation {
			loc.Approximate = approx
			options.UserLocation = loc
		}
	}
	if options.SearchContextSize == "" && !hasLocation {
		return
	}
	params.WebSearchOptions = options
}

func (e *ChatEngram) resolveTemperature(input *Input) float32 {
	temp := e.config.DefaultTemperature
	if input.Temperature != nil {
		temp = *input.Temperature
	}
	return temp
}

func (e *ChatEngram) resolveModel(input *Input) string {
	model := e.config.DefaultModel
	if input.Model != "" {
		model = input.Model
	}
	if e.isAzure {
		if e.azureDep != "" {
			model = e.azureDep
		} else if input.Model != "" {
			model = input.Model
		}
	}
	return model
}

func (e *ChatEngram) shouldUseResponsesAPI(input *Input) bool {
	use := e.config.UseResponsesAPI
	if input != nil && input.UseResponsesAPI != nil {
		use = *input.UseResponsesAPI
	}
	return use
}

func (e *ChatEngram) buildToolParams(
	specs []cfg.ToolSpec,
) ([]openai.ChatCompletionToolUnionParam, map[string]openai.ChatCompletionToolUnionParam) {
	if len(specs) == 0 {
		return nil, nil
	}
	tools := make([]openai.ChatCompletionToolUnionParam, 0, len(specs))
	registry := make(map[string]openai.ChatCompletionToolUnionParam, len(specs))
	for _, t := range specs {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		fn := shared.FunctionDefinitionParam{
			Name:       name,
			Parameters: shared.FunctionParameters(t.Parameters),
		}
		if t.Description != "" {
			fn.Description = openai.String(t.Description)
		}
		tool := openai.ChatCompletionFunctionTool(fn)
		tools = append(tools, tool)
		registry[name] = tool
	}
	if len(tools) == 0 {
		return nil, nil
	}
	return tools, registry
}

func (e *ChatEngram) applyToolChoice(params *openai.ChatCompletionNewParams, choice string) {
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case toolChoiceRequired:
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.Opt(string(openai.ChatCompletionToolChoiceOptionAutoRequired)),
		}
	case toolChoiceNone:
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.Opt(string(openai.ChatCompletionToolChoiceOptionAutoNone)),
		}
	case toolChoiceAuto, "":
		// default behavior, rely on OpenAI to decide
	default:
		if strings.HasPrefix(strings.ToLower(choice), "function:") {
			name := strings.TrimSpace(choice[len("function:"):])
			if name != "" {
				params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
					OfFunctionToolChoice: &openai.ChatCompletionNamedToolChoiceParam{
						Function: openai.ChatCompletionNamedToolChoiceFunctionParam{Name: name},
						Type:     constant.ValueOf[constant.Function](),
					},
				}
			}
		}
	}
}

func (e *ChatEngram) applyAllowedTools(
	params *openai.ChatCompletionNewParams,
	allowed *cfg.AllowedToolsConfig,
	registry map[string]openai.ChatCompletionToolUnionParam,
	logger *slog.Logger,
) {
	if allowed == nil || len(allowed.Tools) == 0 || len(registry) == 0 {
		return
	}
	mode := strings.ToLower(strings.TrimSpace(allowed.Mode))
	if mode != toolChoiceRequired {
		mode = toolChoiceAuto
	}
	spec := openai.ChatCompletionAllowedToolsParam{
		Mode: openai.ChatCompletionAllowedToolsMode(mode),
	}
	for _, name := range allowed.Tools {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		tool, ok := registry[name]
		if !ok {
			if logger != nil {
				logger.Debug("allowed tool skipped; no matching definition", "tool", name)
			}
			continue
		}
		if fn := tool.GetFunction(); fn != nil {
			spec.Tools = append(spec.Tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": fn.Name,
				},
			})
		}
	}
	if len(spec.Tools) == 0 {
		return
	}
	params.ToolChoice = openai.ToolChoiceOptionAllowedTools(spec)
}

func (e *ChatEngram) createChatCompletion(
	ctx context.Context,
	params openai.ChatCompletionNewParams,
) (*openai.ChatCompletion, error) {
	resp, err := e.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (e *ChatEngram) mapResponseToOutput(resp *openai.ChatCompletion) *Output {
	if resp == nil || len(resp.Choices) == 0 {
		return &Output{Text: "", Provider: e.providerName()}
	}
	textParts := make([]string, 0, len(resp.Choices))
	toolCalls := make([]ToolCall, 0, len(resp.Choices))
	funcType := string(constant.ValueOf[constant.Function]())
	for _, choice := range resp.Choices {
		if content := strings.TrimSpace(choice.Message.Content); content != "" {
			textParts = append(textParts, choice.Message.Content)
		}
		for _, call := range choice.Message.ToolCalls {
			if call.Type != funcType {
				continue
			}
			fn := call.AsFunction()
			toolCalls = append(toolCalls, ToolCall{
				Name:      fn.Function.Name,
				Arguments: fn.Function.Arguments,
			})
		}
	}
	out := &Output{
		Text:     strings.Join(textParts, "\n\n"),
		Model:    resp.Model,
		Provider: e.providerName(),
	}

	if len(toolCalls) > 0 {
		out.ToolCalls = toolCalls
		e.enrichToolOutputs(out)
	}
	return out
}

func (e *ChatEngram) providerName() string {
	if e.isAzure {
		return "azure-openai"
	}
	return "openai"
}

func (e *ChatEngram) enrichToolOutputs(out *Output) {
	if out == nil || e.config == nil || len(out.ToolCalls) == 0 {
		return
	}
	for _, call := range out.ToolCalls {
		if e.config.StructuredToolName == call.Name {
			var structured any
			if err := json.Unmarshal([]byte(call.Arguments), &structured); err == nil {
				out.Structured = structured
			}
		}
		if e.config.DispatchTools == nil {
			continue
		}
		if storyName, ok := e.config.DispatchTools[call.Name]; ok {
			var maybeAR ActionRequest
			err := json.Unmarshal([]byte(call.Arguments), &maybeAR)
			if err == nil && strings.TrimSpace(maybeAR.StoryName) != "" {
				out.ActionRequest = &maybeAR
				continue
			}
			var args any
			if err := json.Unmarshal([]byte(call.Arguments), &args); err == nil {
				out.ActionRequest = &ActionRequest{StoryName: storyName, Inputs: args}
			} else {
				out.ActionRequest = &ActionRequest{StoryName: storyName}
			}
		}
	}
}

func cloneMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return make(map[string]string)
	}
	cloned := make(map[string]string, len(in))
	for k, v := range in {
		trimmed := strings.TrimSpace(k)
		if trimmed == "" {
			continue
		}
		cloned[trimmed] = v
	}
	return cloned
}

func resolveSecret(secrets *sdkengram.Secrets, keys ...string) string {
	if secrets == nil {
		return ""
	}
	for _, key := range keys {
		if val, ok := secrets.Get(key); ok {
			if trimmed := strings.TrimSpace(val); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func firstNonEmptyPrompt(input *Input) string {
	if input == nil {
		return ""
	}
	if prompt := resolvePromptString(input.UserPrompt); prompt != "" {
		return prompt
	}
	for i := len(input.History) - 1; i >= 0; i-- {
		msg := input.History[i]
		if strings.EqualFold(msg.Role, "user") {
			content := strings.TrimSpace(msg.Content)
			if content != "" {
				return content
			}
		}
	}
	return ""
}

func firstFloat(value *float32, fallback float32) float32 {
	if value != nil {
		return *value
	}
	return fallback
}

func firstInt(value *int, fallback int) int {
	if value != nil {
		return *value
	}
	return fallback
}

func resolveToolChoice(input *Input, cfgIn *cfg.Config) string {
	if input != nil {
		if choice := strings.TrimSpace(input.ToolChoice); choice != "" {
			return choice
		}
		if choice := strings.TrimSpace(input.FunctionCall); choice != "" {
			return choice
		}
	}
	if cfgIn != nil {
		if choice := strings.TrimSpace(cfgIn.ToolChoice); choice != "" {
			return choice
		}
		if choice := strings.TrimSpace(cfgIn.FunctionCall); choice != "" {
			return choice
		}
	}
	return ""
}

func mergeToolSpecs(tools, functions []cfg.ToolSpec) []cfg.ToolSpec {
	if len(tools) == 0 && len(functions) == 0 {
		return nil
	}
	merged := make([]cfg.ToolSpec, 0, len(tools)+len(functions))
	seen := make(map[string]struct{}, len(tools)+len(functions))
	appendUnique := func(values []cfg.ToolSpec) {
		for _, spec := range values {
			name := strings.TrimSpace(spec.Name)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			merged = append(merged, spec)
		}
	}
	appendUnique(tools)
	appendUnique(functions)
	return merged
}

func firstInt64(value *int64, fallback int64) *int64 {
	if value != nil {
		return value
	}
	if fallback == 0 {
		return nil
	}
	copy := fallback
	return &copy
}

func firstBool(value *bool, fallback *bool) *bool {
	if value != nil {
		return value
	}
	return fallback
}

func firstString(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}

func resolveStopSequences(input []string, defaults []string) []string {
	if len(input) > 0 {
		return normalizeStringList(input)
	}
	return normalizeStringList(defaults)
}

func resolveModalities(input []string, defaults []string) []string {
	if len(input) > 0 {
		return normalizeStringList(input)
	}
	return normalizeStringList(defaults)
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		set[v] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

func containsString(values []string, target string) bool {
	for _, v := range values {
		if strings.EqualFold(v, target) {
			return true
		}
	}
	return false
}

func mergeStringMap(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		if strings.TrimSpace(k) == "" {
			continue
		}
		merged[k] = v
	}
	for k, v := range override {
		if strings.TrimSpace(k) == "" {
			continue
		}
		merged[k] = v
	}
	return merged
}

func resolveLogitBias(base map[string]int64, override map[string]int64) map[string]int64 {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := make(map[string]int64, len(base)+len(override))
	for k, v := range base {
		if strings.TrimSpace(k) == "" {
			continue
		}
		merged[k] = v
	}
	for k, v := range override {
		if strings.TrimSpace(k) == "" {
			continue
		}
		merged[k] = v
	}
	return merged
}

func buildResponseFormatUnion(spec *cfg.ResponseFormat) (*openai.ChatCompletionNewParamsResponseFormatUnion, error) {
	if spec == nil || strings.TrimSpace(spec.Type) == "" {
		return nil, nil
	}
	switch strings.ToLower(spec.Type) {
	case "text":
		union := openai.ChatCompletionNewParamsResponseFormatUnion{
			OfText: &shared.ResponseFormatTextParam{Type: constant.ValueOf[constant.Text]()},
		}
		return &union, nil
	case "json_object":
		union := openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{Type: constant.ValueOf[constant.JSONObject]()},
		}
		return &union, nil
	case "json_schema":
		if spec.JSONSchema == nil || strings.TrimSpace(spec.JSONSchema.Name) == "" {
			return nil, fmt.Errorf("json_schema response format requires name and schema")
		}
		schema := shared.ResponseFormatJSONSchemaJSONSchemaParam{
			Name:   spec.JSONSchema.Name,
			Schema: spec.JSONSchema.Schema,
		}
		if desc := strings.TrimSpace(spec.JSONSchema.Description); desc != "" {
			schema.Description = openai.String(desc)
		}
		if spec.JSONSchema.Strict != nil {
			schema.Strict = openai.Bool(*spec.JSONSchema.Strict)
		}
		union := openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				Type:       constant.ValueOf[constant.JSONSchema](),
				JSONSchema: schema,
			},
		}
		return &union, nil
	default:
		return nil, fmt.Errorf("unsupported response format type %q", spec.Type)
	}
}

func toAudioFormat(value string) (openai.ChatCompletionAudioParamFormat, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "wav":
		return openai.ChatCompletionAudioParamFormatWAV, nil
	case "aac":
		return openai.ChatCompletionAudioParamFormatAAC, nil
	case "mp3":
		return openai.ChatCompletionAudioParamFormatMP3, nil
	case "flac":
		return openai.ChatCompletionAudioParamFormatFLAC, nil
	case "opus":
		return openai.ChatCompletionAudioParamFormatOpus, nil
	case "pcm16":
		return openai.ChatCompletionAudioParamFormatPcm16, nil
	default:
		return "", fmt.Errorf("unsupported audio format %q", value)
	}
}

func toAudioVoice(value string) (openai.ChatCompletionAudioParamVoice, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "alloy":
		return openai.ChatCompletionAudioParamVoiceAlloy, nil
	case "ash":
		return openai.ChatCompletionAudioParamVoiceAsh, nil
	case "ballad":
		return openai.ChatCompletionAudioParamVoiceBallad, nil
	case "coral":
		return openai.ChatCompletionAudioParamVoiceCoral, nil
	case "echo":
		return openai.ChatCompletionAudioParamVoiceEcho, nil
	case "sage":
		return openai.ChatCompletionAudioParamVoiceSage, nil
	case "shimmer":
		return openai.ChatCompletionAudioParamVoiceShimmer, nil
	case "verse":
		return openai.ChatCompletionAudioParamVoiceVerse, nil
	case "marin":
		return openai.ChatCompletionAudioParamVoiceMarin, nil
	case "cedar":
		return openai.ChatCompletionAudioParamVoiceCedar, nil
	default:
		return "", fmt.Errorf("unsupported audio voice %q", value)
	}
}
