package auth

import (
	"context"

	"github.com/dsteiman/tickets-general/backend/internal/domain"
)

type ctxKey int

const (
	userKey ctxKey = iota
	sessionKey
	impersonatorKey
)

// ImpersonatorContext carries who-started-the-impersonation, when set. Authz
// still runs against the effective user — this is pure audit/UI plumbing.
type ImpersonatorContext struct {
	UserID string // original global_admin's id
	Email  string
}

// WithSessionUser puts the authenticated session+user pair into the request context.
func WithSessionUser(ctx context.Context, sw domain.SessionWithUser) context.Context {
	ctx = context.WithValue(ctx, userKey, sw.User)
	ctx = context.WithValue(ctx, sessionKey, sw.Session)
	if sw.ImpersonatorUserID != nil {
		ctx = context.WithValue(ctx, impersonatorKey, ImpersonatorContext{
			UserID: sw.ImpersonatorUserID.String(),
			Email:  sw.ImpersonatorEmail,
		})
	}
	return ctx
}

// ImpersonatorFromContext returns the original global_admin who initiated
// this impersonation session, when present.
func ImpersonatorFromContext(ctx context.Context) (ImpersonatorContext, bool) {
	imp, ok := ctx.Value(impersonatorKey).(ImpersonatorContext)
	return imp, ok
}

// UserFromContext returns the authenticated user attached by the auth middleware.
// (User{}, false) when there is no authenticated user.
func UserFromContext(ctx context.Context) (domain.User, bool) {
	u, ok := ctx.Value(userKey).(domain.User)
	return u, ok
}

// SessionFromContext returns the active session attached by the auth middleware.
func SessionFromContext(ctx context.Context) (domain.Session, bool) {
	s, ok := ctx.Value(sessionKey).(domain.Session)
	return s, ok
}
