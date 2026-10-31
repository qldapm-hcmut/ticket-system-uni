package application

import (
	"context"
	"errors"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/shared/kernel"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrSelfCheckin        = errors.New("self check-in forbidden")
	ErrCheckinWindow      = errors.New("check-in window closed")
	ErrCheckinEligibility = errors.New("not eligible for check-in")
)

type AlreadyCheckedIn struct {
	At time.Time
	By string
}

func (e *AlreadyCheckedIn) Error() string { return "already checked in" }

type CheckinInput struct {
	Method         string `json:"method"`
	Code           string `json:"checkinCode"`
	RegistrationID string `json:"registrationId"`
	Note           string `json:"note"`
}
type CheckinResult struct {
	RegistrationID string        `json:"registrationId"`
	Method         string        `json:"method"`
	CheckedInAt    time.Time     `json:"checkedInAt"`
	CheckedInBy    CheckinPerson `json:"checkedInBy"`
	Student        CheckinPerson `json:"student"`
}
type CheckinPerson struct {
	ID          string `json:"id,omitempty"`
	FullName    string `json:"fullName"`
	StudentCode string `json:"studentCode,omitempty"`
}
type CheckinStore interface {
	CheckIn(context.Context, string, string, CheckinInput) (CheckinResult, error)
	UndoCheckIn(context.Context, string, string) error
}

// CheckinState contains persistence-independent facts required by the policy.
type CheckinState struct {
	StudentID, EventStatus, RegistrationStatus string
	StartAt, EndAt                             time.Time
}
type CheckinPolicy struct{ Clock kernel.Clock }

func (p CheckinPolicy) ValidateInput(in CheckinInput) error {
	switch in.Method {
	case "CODE":
		code := strings.ToUpper(strings.ReplaceAll(in.Code, "-", ""))
		if len(code) != 8 || in.RegistrationID != "" || in.Note != "" {
			return ErrValidation
		}
		for _, c := range code {
			if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
				return ErrValidation
			}
		}
	case "MANUAL":
		if in.RegistrationID == "" || in.Code != "" || strings.TrimSpace(in.Note) == "" || utf8.RuneCountInString(in.Note) > 255 {
			return ErrValidation
		}
	default:
		return ErrValidation
	}
	return nil
}
func (p CheckinPolicy) Authorize(actor string, allowed bool, state CheckinState, undo bool) error {
	if !allowed {
		return ErrNotFound
	}
	if state.StudentID == actor {
		return ErrSelfCheckin
	}
	if undo {
		return nil
	}
	if state.EventStatus != "PUBLISHED" || state.RegistrationStatus != "REGISTERED" {
		return ErrCheckinEligibility
	}
	now := p.Clock.Now()
	if now.Before(state.StartAt.Add(-time.Hour)) || !now.Before(state.EndAt) {
		return ErrCheckinWindow
	}
	return nil
}
