package rbac

import (
	"context"
	"strings"
)

type Checker struct {
	RolePermissions map[string][]string
}

func NewChecker(rp map[string][]string) *Checker {
	if rp == nil {
		rp = RolePermissions
	}
	return &Checker{RolePermissions: rp}
}

func (c *Checker) Has(role, perm string) bool {
	perms, ok := c.RolePermissions[role]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == "*" || matchPerm(p, perm) {
			return true
		}
	}
	return false
}

func (c *Checker) Any(role string, perms ...string) bool {
	for _, p := range perms {
		if c.Has(role, p) {
			return true
		}
	}
	return false
}

func (c *Checker) All(role string, perms ...string) bool {
	for _, p := range perms {
		if !c.Has(role, p) {
			return false
		}
	}
	return true
}

func matchPerm(pattern, perm string) bool {
	if pattern == "*" || pattern == perm {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(perm, strings.TrimSuffix(pattern, "*"))
	}
	return false
}

// ---- role in context ----

type ctxKey struct{}

var ctxKeyRole = ctxKey{}

func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, ctxKeyRole, role)
}

func RoleFromContext(ctx context.Context) string {
	if v := ctx.Value(ctxKeyRole); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
