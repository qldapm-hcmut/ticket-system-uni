package application

import "errors"

var (
	ErrUnauthenticated  = errors.New("unauthenticated")
	ErrNotFound         = errors.New("auth record not found or not eligible")
	ErrIdentityConflict = errors.New("auth identity conflict")
)
