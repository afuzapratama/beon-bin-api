package requestcontext

import (
	"context"
	"testing"
)

func TestRequestIDContext(t *testing.T) {
	ctx := context.Background()
	if id := RequestID(ctx); id != "" {
		t.Fatalf("empty context request ID = %q", id)
	}
	ctx = WithRequestID(ctx, "request-123")
	if id := RequestID(ctx); id != "request-123" {
		t.Fatalf("request ID = %q", id)
	}
}
