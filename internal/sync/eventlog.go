package syncx

import "context"

type Event struct {
	Offset int64  // monotonic
	Type   string // e.g., AttemptSubmitted
	Key    string // natural key (attemptID)
	Data   []byte // json payload
}

type EventRepo interface {
	Append(ctx context.Context, ev Event) error
	ListAfter(ctx context.Context, after int64, limit int) ([]Event, error)
}
