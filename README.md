## OpenAI Chat Engram

Chat engram for building conversational steps with OpenAI (and Azure OpenAI). Supports batch (Kubernetes Job) and streaming (Deployment/StatefulSet) modes, function tools, structured outputs, and story dispatch.

### Features
- Chat Completions API (models like `gpt-4*`, `gpt-3.5-turbo`, `gpt-4o*`)
- Per-request overrides for model/temperature/system prompt
- Function tools (function calling)
- Structured outputs via a designated tool
- Tool dispatch to stories via `actionRequest`
- Azure OpenAI support via env-prefixed secrets

### Secrets
- OpenAI (required for OpenAI API):
  - `OPENAI_API_KEY` (required)
  - `OPENAI_BASE_URL` (optional)
  - `OPENAI_ORG_ID` (optional)
- Azure OpenAI (optional; enables Azure mode when `AZURE_ENDPOINT` is present):
  - `AZURE_API_KEY` (required in Azure mode)
  - `AZURE_ENDPOINT` (required in Azure mode)
  - `AZURE_API_VERSION` (default: `2024-08-01-preview`)
  - `AZURE_DEPLOYMENT` (maps requested model → deployment)

Engram template defines two logical secrets `openai` and `azure`, both with `mountType: env` and prefixes `OPENAI_` and `AZURE_` respectively.

### Configuration (Engram.spec.with)
```yaml
defaultModel: gpt-4o-mini
defaultTemperature: 0.2
defaultSystemPrompt: "You are a helpful assistant."

# Optional function tools exposed to the model
tools:
  - name: run_story
    description: "Trigger a follow-up story with inputs"
    parameters:
      type: object
      properties:
        storyName: { type: string, required: true }
        inputs: { type: object }
  - name: extract_fields
    description: "Extract structured fields from user input"
    parameters:
      type: object
      properties:
        title: { type: string }
        priority: { type: string, enum: [low, medium, high] }
        tags: { type: array, items: { type: string } }

# Name of the tool whose arguments should be returned as structured output
structuredToolName: extract_fields

# Map tool → storyName. If the model calls one of these tools,
# the engram emits actionRequest automatically.
dispatchTools:
  run_story: "create-ticket-story"
```

### Inputs (StepRun.spec.input → provided to the engram)
```json
{
  "history": [
    { "role": "user", "content": "Create a ticket for server outage." }
  ],
  "systemPrompt": "Be concise.",
  "userPrompt": "Priority should be high and tags: ops, urgent",
  "temperature": 0.1,
  "model": "gpt-4o-mini"
}
```

Supported fields:
- `history`: prior messages (`role` ∈ `system|developer|user|assistant`; `developer` is mapped to system for Chat Completions)
- `systemPrompt`, `userPrompt`, `developerPrompt`, `assistantPrompt`
- `temperature`, `model`

### Outputs
```json
{
  "text": "Created ticket with title 'Server outage' and priority 'high'.",
  "structured": {
    "title": "Server outage",
    "priority": "high",
    "tags": ["ops", "urgent"]
  },
  "actionRequest": {
    "storyName": "create-ticket-story",
    "inputs": {
      "title": "Server outage",
      "priority": "high",
      "tags": ["ops", "urgent"]
    }
  },
  "toolCalls": [
    {
      "name": "extract_fields",
      "arguments": "{\"title\":\"Server outage\",\"priority\":\"high\",\"tags\":[\"ops\",\"urgent\"]}"
    },
    {
      "name": "run_story",
      "arguments": "{\"storyName\":\"create-ticket-story\",\"inputs\":{...}}"
    }
  ]
}
```

Notes:
- `structured` appears when the model calls the tool named in `structuredToolName`; it contains the parsed JSON arguments.
- `actionRequest` appears when the model calls a tool listed in `dispatchTools`. If the tool arguments already contain `{storyName, inputs}`, it passes through; else the arguments are wrapped under the configured `storyName`.
- `toolCalls` includes all raw function calls for advanced routing or debugging.

### Batch vs Streaming
- Batch (Job): default; processes a single input and writes the output to the StepRun.
- Streaming (Deployment/StatefulSet): processes a stream of inputs and emits an output per input message (not token-by-token). Use when you need low-latency multi-turn chat within a long-running service.

### Implementation details
- Client: currently uses `github.com/sashabaranov/go-openai` Chat Completions.
- Planned: optional Responses API support using the official library to unify multimodal and structured outputs.

### References
- OpenAI official Go SDK (Responses, Realtime, Webhooks, Azure helpers): `https://github.com/openai/openai-go`
- n8n OpenAI node capabilities (for parity context): `https://docs.n8n.io/integrations/builtin/app-nodes/n8n-nodes-langchain.openai/`


