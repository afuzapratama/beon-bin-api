package requestcontext

import "context"

type requestIDKey struct{}

// WithRequestID returns a context carrying the request correlation ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the correlation ID, or an empty string outside an HTTP request.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
