package application

import "context"

// UnitOfWork runs related repository operations atomically.
// A nil callback result commits; an error rolls back the operations.
type UnitOfWork interface {
	Do(context.Context, func(context.Context, Repository) error) error
}
