package application

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("report event not found")
var ErrValidation = errors.New("invalid export options")

type Summary struct {
	Capacity       int32            `json:"capacity"`
	Registered     int64            `json:"registered"`
	Waitlisted     int64            `json:"waitlisted"`
	Cancelled      int64            `json:"cancelled"`
	Promoted       int64            `json:"promotedFromWaitlist"`
	CheckedIn      int64            `json:"checkedIn"`
	AttendanceRate float64          `json:"attendanceRate"`
	ByMethod       map[string]int64 `json:"checkInByMethod"`
	GeneratedAt    time.Time        `json:"generatedAt"`
}

type Store interface {
	Summary(context.Context, string, string) (Summary, error)
	Export(context.Context, string, string, string) ([]byte, error)
}
type Service struct{ store Store }

func New(store Store) *Service { return &Service{store} }
func (s *Service) Summary(ctx context.Context, actor, event string) (Summary, error) {
	return s.store.Summary(ctx, actor, event)
}
func (s *Service) Export(ctx context.Context, actor, event, kind, format string) ([]byte, error) {
	if (kind != "registrations" && kind != "attendance") || (format != "" && format != "csv") {
		return nil, ErrValidation
	}
	return s.store.Export(ctx, actor, event, kind)
}
