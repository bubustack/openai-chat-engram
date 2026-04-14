package engram

import (
	"testing"

	sdkengram "github.com/bubustack/bubu-sdk-go/engram"
)

func TestStreamInputBytesPrefersInputsThenPayloadThenBinary(t *testing.T) {
	msg := sdkengram.NewInboundMessage(sdkengram.StreamMessage{
		Inputs:  []byte(`{"userPrompt":"inputs"}`),
		Payload: []byte(`{"userPrompt":"payload"}`),
		Binary: &sdkengram.BinaryFrame{
			Payload:  []byte(`{"userPrompt":"binary"}`),
			MimeType: "application/json",
		},
	})
	if got := string(streamInputBytes(msg)); got != `{"userPrompt":"inputs"}` {
		t.Fatalf("expected inputs to win, got %q", got)
	}
}

func TestStreamInputBytesPrefersPayloadOverBinary(t *testing.T) {
	msg := sdkengram.NewInboundMessage(sdkengram.StreamMessage{
		Payload: []byte(`{"userPrompt":"payload"}`),
		Binary: &sdkengram.BinaryFrame{
			Payload:  []byte(`{"userPrompt":"binary"}`),
			MimeType: "application/json",
		},
	})
	if got := string(streamInputBytes(msg)); got != `{"userPrompt":"payload"}` {
		t.Fatalf("expected payload to win, got %q", got)
	}
}
