package application

import "errors"

var (
	ErrNotFound         = errors.New("auth record not found or not eligible")
	ErrIdentityConflict = errors.New("auth identity conflict")
)
