package application

import (
	"errors"
	"testing"
	"time"
)

type checkinClock struct{ now time.Time }

func (c checkinClock) Now() time.Time { return c.now }
func TestCheckinPolicy(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	p := CheckinPolicy{Clock: checkinClock{now}}
	for _, tc := range []struct {
		name       string
		start, end time.Time
		want       error
	}{
		{"before opening", now.Add(time.Hour + time.Nanosecond), now.Add(3 * time.Hour), ErrCheckinWindow},
		{"exact opening", now.Add(time.Hour), now.Add(3 * time.Hour), nil},
		{"before end", now.Add(-time.Hour), now.Add(time.Nanosecond), nil},
		{"exact end", now.Add(-time.Hour), now, ErrCheckinWindow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := p.Authorize("staff", true, CheckinState{StudentID: "student", EventStatus: "PUBLISHED", RegistrationStatus: "REGISTERED", StartAt: tc.start, EndAt: tc.end}, false)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
	for _, undo := range []bool{false, true} {
		if err := p.Authorize("staff", true, CheckinState{StudentID: "staff"}, undo); !errors.Is(err, ErrSelfCheckin) {
			t.Fatal(err)
		}
	}
	for _, in := range []CheckinInput{{Method: "MANUAL", RegistrationID: "id"}, {Method: "CODE", Code: "bad"}, {Method: "QR"}, {Method: "MANUAL", RegistrationID: "id", Note: string(make([]byte, 256))}} {
		if err := p.ValidateInput(in); !errors.Is(err, ErrValidation) {
			t.Fatalf("accepted %+v", in)
		}
	}
}
