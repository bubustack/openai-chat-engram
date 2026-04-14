# 💬 OpenAI Chat Engram

A production-ready Engram for calling OpenAI (and Azure OpenAI) Chat Completions inside bobrapet workflows. It supports rich prompting, function tools, structured outputs, and story dispatch, and runs as either a batch Job or a streaming Deployment.

## 🌟 Highlights

- **OpenAI & Azure OpenAI** – Auto-detects Azure when `AZURE_ENDPOINT` is present, otherwise uses OpenAI’s public API.
- **Function tools + structured outputs** – Register tools via `spec.with.tools`; pipe structured arguments back through `structuredToolName`.
- **Story dispatch** – Map tool invocations to downstream `StoryRun`s using `dispatchTools`.
- **Dual mode** – Same implementation powers batch step runs and streaming deployments with backpressure.
- **Per-request overrides** – Customize `model`, `temperature`, prompts, or history per invocation without editing configuration.

## 🚀 Quick Start

```bash
make lint
go test ./...
make docker-build
```

Bundle the image in your bobrapet release or run locally with `make run` (requires bobrapet execution environment variables).

## ⚙️ Configuration (`Engram.spec.with`)

| Field | Type | Description | Default |
| --- | --- | --- | --- |
| `defaultModel` | `string` | Chat model to use when inputs omit `model`. | `gpt-3.5-turbo` |
| `defaultTemperature` | `float32` | Base temperature for sampling. | `0.7` |
| `defaultSystemPrompt` | `string` | Global system prompt for the Engram. | `"You are a helpful assistant."` |
| `defaultTopP` | `float32` | Default nucleus sampling probability mass. | `0.9` |
| `defaultMaxTokens` | `int` | Legacy total-token cap. | `0` |
| `defaultMaxCompletionTokens` | `int` | Preferred completion-token cap. | `0` |
| `defaultPresencePenalty` | `float32` | Default presence penalty when no override is provided. | `0` |
| `defaultFrequencyPenalty` | `float32` | Default frequency penalty when no override is provided. | `0` |
| `defaultReasoningEffort` | `string` | Reasoning hint for supported models (`minimal`, `low`, `medium`, `high`). | unset |
| `defaultServiceTier` | `string` | Preferred OpenAI service tier (`auto`, `default`, `flex`, `scale`, `priority`). | unset |
| `defaultVerbosity` | `string` | Verbosity hint for supported models (`low`, `medium`, `high`). | unset |
| `defaultModalities` | `[]string` | Default output modalities such as `text` or `audio`. | unset |
| `defaultStopSequences` | `[]string` | Sequences that stop generation when encountered. | unset |
| `defaultStore` | `bool` | Allow OpenAI to store responses by default. | unset |
| `defaultParallelToolCalls` | `bool` | Enable parallel tool calls by default. | unset |
| `defaultLogprobs` | `bool` | Request token log probabilities by default. | unset |
| `defaultTopLogprobs` | `int` | Number of logprob candidates to include when logprobs are enabled. | unset |
| `defaultChoices` | `int` | Default value for the OpenAI `n` parameter. | unset |
| `defaultSeed` | `int` | Deterministic seed for supported models. | unset |
| `defaultPromptCacheKey` | `string` | Prompt cache key forwarded to OpenAI. | unset |
| `defaultSafetyIdentifier` | `string` | Default safety identifier for abuse monitoring. | unset |
| `defaultUser` | `string` | Default end-user identifier sent to OpenAI. | unset |
| `defaultMetadata` | `map[string]string` | Default metadata map attached to each request. | unset |
| `defaultLogitBias` | `map[int]int` | Default logit bias map (token ID to bias). | unset |
| `responseFormat` | `object` | Structured output configuration with `type` and optional `jsonSchema.{name,description,strict,schema}`. | unset |
| `audio` | `object` | Default multimodal audio response config with `format` and `voice`. | unset |
| `tools` | `[]ToolSpec` | Function tools exposed to the model. | `[]` |
| `functions` | `[]ToolSpec` | Legacy alias for `tools`; merged and deduplicated by tool name. | `[]` |
| `toolChoice` | `string` | `auto`, `required`, `none`, or `function:<name>`. Controls tool selection hints. | `auto` |
| `functionCall` | `string` | Legacy alias for `toolChoice`. | `""` |
| `allowedTools` | `object` | Restrict tool use to a curated set via `mode` and explicit tool names. | unset |
| `webSearch` | `object` | Default web search options including `searchContextSize` and `userLocation`. | unset |
| `structuredToolName` | `string` | Tool whose arguments are returned in `Result.Structured`. | `""` |
| `dispatchTools` | `map[string]string` | Map `toolName → storyName` for auto-dispatch when the tool doesn’t return its own `storyName`. | `{}` |
| `useResponsesAPI` | `bool` | Route requests through the Responses API instead of Chat Completions. | `false` |

Example:

```yaml
defaultModel: gpt-4o-mini
defaultTemperature: 0.2
tools:
  - name: run_story
    description: Trigger a follow-up story with inputs
    parameters:
      type: object
      properties:
        storyName: { type: string }
        inputs: { type: object }
structuredToolName: extract_fields
dispatchTools:
  run_story: create-ticket-story
```

### Structured Tool and Dispatch Behavior

When `structuredToolName` is set, the model's call to that tool extracts its JSON arguments into `Result.Structured` instead of executing the tool. This is useful for structured data extraction (e.g., extracting fields from a conversation).

When `dispatchTools` is configured, a tool call matching a key in the map automatically triggers the mapped Story. The tool's arguments become the StoryRun inputs. If both `structuredToolName` and `dispatchTools` reference the same tool, structured extraction takes precedence.

```json
// Example output when structuredToolName = "extract_fields"
{
  "structured": {
    "customer_name": "Alice",
    "issue_type": "billing",
    "priority": "high"
  },
  "text": "I've extracted the relevant fields from the conversation."
}
```

## 🔐 Secrets

| Mode | Environment Prefix | Required Keys | Optional Keys |
| --- | --- | --- | --- |
| OpenAI | `OPENAI_` | `API_KEY` | `BASE_URL`, `ORG_ID`, `PROJECT_ID` |
| Azure OpenAI | `AZURE_` | `API_KEY`, `ENDPOINT` | `API_VERSION` (default `2024-06-01`), `DEPLOYMENT` |

Secrets are mounted via `mountType: env` in the EngramTemplate.

## 📥 Inputs

Inputs are provided through the StepRun `spec.inputs` or streaming `StreamMessage`. Supported fields:

| Field | Description |
| --- | --- |
| `history` | Array of prior messages (`role` ∈ `system`, `developer`, `user`, `assistant`). `developer` is mapped to system. |
| `systemPrompt`, `developerPrompt`, `userPrompt`, `assistantPrompt` | Prompt fragments merged in this order. |
| `temperature` | Overrides temperature for this request. |
| `model` | Overrides model for this request. |

Advanced request-time overrides mirror the template schema. You can override
`topP`, `maxTokens`, `maxCompletionTokens`, `presencePenalty`, `frequencyPenalty`,
`stop`, `modalities`, `store`, `parallelToolCalls`, `logprobs`, `topLogprobs`,
`seed`, `serviceTier`, `reasoningEffort`, `verbosity`, `responseFormat`,
`metadata`, `logitBias`, `audio`, `promptCacheKey`, `safetyIdentifier`, `user`,
`tools`, `functions`, `toolChoice`, `functionCall`, `choices`, `allowedTools`,
`useResponsesAPI`, `prediction`, and `webSearch` per invocation.

Example payload:

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

## 📤 Outputs

The Engram returns:

| Field | Description |
| --- | --- |
| `text` | Final model message concatenated as plain text. |
| `structured` | Parsed JSON arguments from the tool named in `structuredToolName`. |
| `actionRequest` | When a tool listed in `dispatchTools` fires; contains `storyName` and optional `inputs`. |
| `toolCalls` | Raw tool call invocations (`name`, `arguments` as JSON). |

## 🔄 Streaming Mode

Streaming consumes `engram.InboundMessage` on input and emits
`engram.StreamMessage` on output:

- If `Inputs` is set, it is decoded into the request payload described above.
- If `Inputs` is empty, the Engram attempts to decode `Payload` as the same structure.
- `Metadata` is copied and enriched with `type`, `provider`, and `model` fields.
- Call `msg.Done()` after successful handling so ordered/replay-capable transports can advance acknowledgement state.

Responses are emitted as `StreamMessage` with `Payload` containing the
JSON-encoded output map shown above, and the same bytes mirrored into `Binary`
with `MimeType: application/json`.

## 🧪 Local Development

- `make lint` – GolangCI-Lint using the shared config.
- `go test ./...` – Compile and run unit tests.
- `make run` – Execute locally against the bobrapet runtime environment.
- `make docker-build` – Multi-stage image build (honours `IMG`).

## 🤝 Community & Support

- [Contributing](./CONTRIBUTING.md)
- [Support](./SUPPORT.md)
- [Security Policy](./SECURITY.md)
- [Code of Conduct](./CODE_OF_CONDUCT.md)
- [Discord](https://discord.gg/dysrB7D8H6)


## 📄 License

Copyright 2025 BubuStack.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
