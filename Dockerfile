# --- Build Stage ---
FROM golang:1.24-alpine AS builder

# Install git, which is required for Go modules to fetch dependencies.
RUN apk add --no-cache git

WORKDIR /src

# Copy the entire project context.
# This ensures that the local 'replace' directives in go.mod work correctly.
COPY . .

RUN go mod download

# Build the binary from within the engram's directory.
# The Go toolchain will find the parent go.mod and handle the local SDK dependency.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o openai-chat main.go

# --- Final Stage ---
FROM gcr.io/distroless/static-debian12

COPY --from=builder /src/openai-chat /openai-chat

# Set the default execution mode to "batch".
# This can be overridden at runtime by setting the environment variable.
ENV BUBU_EXECUTION_MODE="batch"

ENTRYPOINT ["/openai-chat"]