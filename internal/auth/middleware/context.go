package auth

import "context"

type ctxKey string

const ctxKeySub ctxKey = "sub"

func WithSubject(ctx context.Context, sub string) context.Context {
	return context.WithValue(ctx, ctxKeySub, sub)
}

func SubjectFromContext(ctx context.Context) string {
	if v := ctx.Value(ctxKeySub); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
