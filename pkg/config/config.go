package config

// ToolSpec defines a single function tool exposed to the model.
type ToolSpec struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// Config is the static configuration for the chat engram.
type Config struct {
	DefaultModel        string  `json:"defaultModel"`
	DefaultTemperature  float32 `json:"defaultTemperature"`
	DefaultSystemPrompt string  `json:"defaultSystemPrompt"`

	Tools      []ToolSpec `json:"tools"`
	ToolChoice string     `json:"toolChoice"` // auto|required|none

	StructuredToolName string            `json:"structuredToolName"`
	DispatchTools      map[string]string `json:"dispatchTools"`
}
