package exam

import "context"

// Perf is a minimal snapshot of performance that routers can use.
// Populate RawPoints (e.g., # correct so far in the current section/module).
type Perf struct {
	RawPoints float64
}

// Router decides which concrete module ID to deliver next.
// Return "" to let the engine advance sequentially using the policy's placeholder ID.
type Router interface {
	NextModule(ctx context.Context, ex Exam, a Attempt, perf Perf) (nextModuleID string, err error)
}

// ---- Registry ----

type routerRegistry struct {
	m map[string]Router
}

var routers = routerRegistry{m: map[string]Router{}}

// RegisterRouter associates a profile (e.g., "sat.v1") with a Router.
// Typically called from profile packages' init().
func RegisterRouter(profile string, r Router) {
	if profile == "" || r == nil {
		return
	}
	routers.m[profile] = r
}

// RouterForProfile fetches a router for a profile, or nil if none.
func RouterForProfile(profile string) Router {
	return routers.m[profile]
}
