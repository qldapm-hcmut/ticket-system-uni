package application

import (
	shared "github.com/qldapm-hcmut/ticket-system-uni/internal/shared/application"
)

// UnitOfWork runs related repository operations atomically.
// A nil callback result commits; an error rolls back the operations.
type UnitOfWork interface{ shared.UnitOfWork[Repository] }
