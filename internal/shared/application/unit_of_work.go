package application

import "context"

// UnitOfWork executes one application workflow atomically.
// Implementations commit when fn returns nil and roll back on error.
type UnitOfWork[R any] interface {
	Do(context.Context, func(context.Context, R) error) error
}
