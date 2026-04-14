package config

import (
	"slices"
	"strings"
)

// ToolSpec defines a single function tool exposed to the model.
type ToolSpec struct {
	Name        string         `json:"name" mapstructure:"name"`
	Description string         `json:"description,omitempty" mapstructure:"description"`
	Parameters  map[string]any `json:"parameters,omitempty" mapstructure:"parameters"`
}

// WebSearchConfig captures default web-search constraints.
type WebSearchConfig struct {
	SearchContextSize string             `json:"searchContextSize" mapstructure:"searchContextSize"`
	UserLocation      *WebSearchLocation `json:"userLocation" mapstructure:"userLocation"`
}

// WebSearchLocation approximates the caller's location for search tuning.
type WebSearchLocation struct {
	City     string `json:"city" mapstructure:"city"`
	Country  string `json:"country" mapstructure:"country"`
	Region   string `json:"region" mapstructure:"region"`
	Timezone string `json:"timezone" mapstructure:"timezone"`
}

// AllowedToolsConfig constrains which tools may be invoked.
type AllowedToolsConfig struct {
	Mode  string   `json:"mode" mapstructure:"mode"`
	Tools []string `json:"tools" mapstructure:"tools"`
}

// ResponseFormat captures preferred structured output settings.
type ResponseFormat struct {
	Type       string              `json:"type" mapstructure:"type"`
	JSONSchema *ResponseJSONSchema `json:"jsonSchema" mapstructure:"jsonSchema"`
}

// ResponseJSONSchema mirrors OpenAI's structured output schema payload.
type ResponseJSONSchema struct {
	Name        string         `json:"name" mapstructure:"name"`
	Description string         `json:"description" mapstructure:"description"`
	Strict      *bool          `json:"strict" mapstructure:"strict"`
	Schema      map[string]any `json:"schema" mapstructure:"schema"`
}

// AudioConfig describes optional audio output parameters for chat completions.
type AudioConfig struct {
	Format string `json:"format" mapstructure:"format"`
	Voice  string `json:"voice" mapstructure:"voice"`
}

// Config captures static configuration injected via Engram.spec.with.
type Config struct {
	DefaultModel        string  `json:"defaultModel" mapstructure:"defaultModel"`
	DefaultTemperature  float32 `json:"defaultTemperature" mapstructure:"defaultTemperature"`
	DefaultTopP         float32 `json:"defaultTopP" mapstructure:"defaultTopP"`
	DefaultSystemPrompt string  `json:"defaultSystemPrompt" mapstructure:"defaultSystemPrompt"`
	DefaultMaxTokens    int     `json:"defaultMaxTokens" mapstructure:"defaultMaxTokens"`
	//nolint:lll // Field name/tag pair must match the manifest schema.
	DefaultMaxCompletion     int               `json:"defaultMaxCompletionTokens" mapstructure:"defaultMaxCompletionTokens"`
	DefaultPresencePenalty   float32           `json:"defaultPresencePenalty" mapstructure:"defaultPresencePenalty"`
	DefaultFrequencyPenalty  float32           `json:"defaultFrequencyPenalty" mapstructure:"defaultFrequencyPenalty"`
	DefaultReasoningEffort   string            `json:"defaultReasoningEffort" mapstructure:"defaultReasoningEffort"`
	DefaultServiceTier       string            `json:"defaultServiceTier" mapstructure:"defaultServiceTier"`
	DefaultVerbosity         string            `json:"defaultVerbosity" mapstructure:"defaultVerbosity"`
	DefaultModalities        []string          `json:"defaultModalities" mapstructure:"defaultModalities"`
	DefaultStopSequences     []string          `json:"defaultStopSequences" mapstructure:"defaultStopSequences"`
	DefaultStore             *bool             `json:"defaultStore" mapstructure:"defaultStore"`
	DefaultParallelToolCalls *bool             `json:"defaultParallelToolCalls" mapstructure:"defaultParallelToolCalls"`
	DefaultLogprobs          *bool             `json:"defaultLogprobs" mapstructure:"defaultLogprobs"`
	DefaultTopLogprobs       int               `json:"defaultTopLogprobs" mapstructure:"defaultTopLogprobs"`
	DefaultSeed              int64             `json:"defaultSeed" mapstructure:"defaultSeed"`
	DefaultPromptCacheKey    string            `json:"defaultPromptCacheKey" mapstructure:"defaultPromptCacheKey"`
	DefaultSafetyIdentifier  string            `json:"defaultSafetyIdentifier" mapstructure:"defaultSafetyIdentifier"`
	DefaultUser              string            `json:"defaultUser" mapstructure:"defaultUser"`
	DefaultMetadata          map[string]string `json:"defaultMetadata" mapstructure:"defaultMetadata"`
	DefaultLogitBias         map[string]int64  `json:"defaultLogitBias" mapstructure:"defaultLogitBias"`
	DefaultChoices           int               `json:"defaultChoices" mapstructure:"defaultChoices"`
	ResponseFormat           *ResponseFormat   `json:"responseFormat" mapstructure:"responseFormat"`
	Audio                    *AudioConfig      `json:"audio" mapstructure:"audio"`

	Tools        []ToolSpec          `json:"tools" mapstructure:"tools"`
	Functions    []ToolSpec          `json:"functions" mapstructure:"functions"`
	AllowedTools *AllowedToolsConfig `json:"allowedTools" mapstructure:"allowedTools"`
	ToolChoice   string              `json:"toolChoice" mapstructure:"toolChoice"` // auto|required|none|function name
	FunctionCall string              `json:"functionCall" mapstructure:"functionCall"`
	WebSearch    *WebSearchConfig    `json:"webSearch" mapstructure:"webSearch"`

	StructuredToolName string            `json:"structuredToolName" mapstructure:"structuredToolName"`
	DispatchTools      map[string]string `json:"dispatchTools" mapstructure:"dispatchTools"`
	UseResponsesAPI    bool              `json:"useResponsesAPI" mapstructure:"useResponsesAPI"`
}

// Normalize returns a sanitized copy of the config with defaults applied.
func Normalize(cfg Config) Config {
	cfg.DefaultModel = strings.TrimSpace(cfg.DefaultModel)
	cfg.DefaultSystemPrompt = strings.TrimSpace(cfg.DefaultSystemPrompt)
	cfg.DefaultReasoningEffort = strings.TrimSpace(strings.ToLower(cfg.DefaultReasoningEffort))
	cfg.DefaultServiceTier = strings.TrimSpace(strings.ToLower(cfg.DefaultServiceTier))
	cfg.DefaultVerbosity = strings.TrimSpace(strings.ToLower(cfg.DefaultVerbosity))
	cfg.DefaultModalities = normalizeList(cfg.DefaultModalities)
	cfg.DefaultStopSequences = normalizeList(cfg.DefaultStopSequences)
	cfg.ToolChoice = normalizeToolChoice(cfg.ToolChoice)
	cfg.FunctionCall = normalizeToolChoice(cfg.FunctionCall)
	if cfg.ResponseFormat != nil {
		cfg.ResponseFormat.Type = strings.TrimSpace(strings.ToLower(cfg.ResponseFormat.Type))
		if cfg.ResponseFormat.JSONSchema != nil {
			cfg.ResponseFormat.JSONSchema.Name = strings.TrimSpace(cfg.ResponseFormat.JSONSchema.Name)
			cfg.ResponseFormat.JSONSchema.Description = strings.TrimSpace(cfg.ResponseFormat.JSONSchema.Description)
		}
	}
	if cfg.Audio != nil {
		cfg.Audio.Format = strings.TrimSpace(strings.ToLower(cfg.Audio.Format))
		cfg.Audio.Voice = strings.TrimSpace(strings.ToLower(cfg.Audio.Voice))
	}
	if cfg.WebSearch != nil {
		cfg.WebSearch = NormalizeWebSearch(cfg.WebSearch)
	}
	if cfg.AllowedTools != nil {
		cfg.AllowedTools = NormalizeAllowedTools(cfg.AllowedTools)
	}
	return cfg
}

func normalizeToolChoice(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "function:") {
		name := strings.TrimSpace(trimmed[len("function:"):])
		if name == "" {
			return ""
		}
		return "function:" + name
	}
	return lower
}

func normalizeList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{})
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

func NormalizeWebSearch(in *WebSearchConfig) *WebSearchConfig {
	if in == nil {
		return nil
	}
	out := *in
	out.SearchContextSize = strings.TrimSpace(strings.ToLower(out.SearchContextSize))
	if !nullLocation(in.UserLocation) {
		loc := *in.UserLocation
		loc.City = strings.TrimSpace(loc.City)
		loc.Country = strings.ToUpper(strings.TrimSpace(loc.Country))
		loc.Region = strings.TrimSpace(loc.Region)
		loc.Timezone = strings.TrimSpace(loc.Timezone)
		out.UserLocation = &loc
	} else {
		out.UserLocation = nil
	}
	return &out
}

func nullLocation(loc *WebSearchLocation) bool {
	return loc == nil || (strings.TrimSpace(loc.City) == "" &&
		strings.TrimSpace(loc.Country) == "" &&
		strings.TrimSpace(loc.Region) == "" &&
		strings.TrimSpace(loc.Timezone) == "")
}

func NormalizeAllowedTools(cfg *AllowedToolsConfig) *AllowedToolsConfig {
	if cfg == nil {
		return nil
	}
	out := &AllowedToolsConfig{
		Mode:  strings.TrimSpace(strings.ToLower(cfg.Mode)),
		Tools: normalizeList(cfg.Tools),
	}
	switch out.Mode {
	case "auto", "required":
	default:
		out.Mode = "auto"
	}
	if len(out.Tools) == 0 {
		return nil
	}
	return out
}
