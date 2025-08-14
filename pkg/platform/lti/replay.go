// pkg/platform/lti/replay.go
package lti

import "time"

// Replay is used to prevent nonce/jti replays. Implementations should
// mark values as used atomically and respect the TTL hint.
type Replay interface {
	Use(kind, value string, ttl time.Duration) (bool, error)
}

// NoopReplay accepts everything (dev/tests).
type NoopReplay struct{}

func (NoopReplay) Use(kind, value string, ttl time.Duration) (bool, error) { return true, nil }
