package application

import "time"

// Session deliberately excludes the refresh token and its stored hash.
type Session struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
}
