package application

import (
	"context"
	"time"
)

// Repository is transaction-scoped. Callers must not retain it after Do returns.
// Google identities must be verified before invoking identity writes.
// Session methods accept only hashes, never raw refresh tokens.
type Repository interface {
	FindUserBySub(context.Context, string) (User, error)
	FindUserByEmail(context.Context, string) (User, error)
	CreateUser(context.Context, string, string, string) (User, error)
	LinkIdentity(context.Context, string, string) (User, error)
	CreateSession(context.Context, string, string, time.Time) (Session, error)
	// ConsumeSession atomically revokes an active, unexpired session of an active user.
	// Insert its replacement in the same UnitOfWork for refresh rotation.
	ConsumeSession(context.Context, string, time.Time) (Session, error)
	RevokeSession(context.Context, string, string, time.Time) error
}
