package reqid

import "context"

// key is an unexported type to avoid collisions in context values.
type key struct{}

// With returns a new context with the provided request ID attached.
func With(ctx context.Context, id string) context.Context {
    if ctx == nil {
        ctx = context.Background()
    }
    return context.WithValue(ctx, key{}, id)
}

// From extracts the request ID from the context, if present.
func From(ctx context.Context) (string, bool) {
    if ctx == nil {
        return "", false
    }
    v := ctx.Value(key{})
    if v == nil {
        return "", false
    }
    if s, ok := v.(string); ok && s != "" {
        return s, true
    }
    return "", false
}

