# --- Build Stage ---
FROM golang:1.24-alpine AS builder

# Install git, which is required for Go modules to fetch dependencies.
RUN apk add --no-cache git

WORKDIR /src

# Copy the entire project context.
# This ensures that the local 'replace' directives in go.mod work correctly.
COPY ../.. .

# Tidy the modules to ensure the latest local SDK is used.
RUN cd engrams/openai-chat && go mod tidy

# Build the binary from within the engram's directory.
# The Go toolchain will find the parent go.mod and handle the local SDK dependency.
RUN cd engrams/openai-chat && CGO_ENABLED=0 GOOS=linux go build -o /openai-chat .

# --- Final Stage ---
FROM gcr.io/distroless/static-debian12

COPY --from=builder /openai-chat /openai-chat

ENTRYPOINT ["/openai-chat"]
