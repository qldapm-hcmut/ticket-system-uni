package application

import (
	"context"
	"time"
)

// CheckinSummary describes an existing check-in.
type CheckinSummary struct {
	CheckedInAt time.Time `json:"checkedInAt"`
	Method      string    `json:"method"`
}

// EventSummary is the event data shown with a registration.
type EventSummary struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Location string    `json:"location"`
	StartAt  time.Time `json:"startAt"`
	EndAt    time.Time `json:"endAt"`
	Status   string    `json:"status"`
}

// RegistrationView is the persisted state of one registration owned by a student.
type RegistrationView struct {
	ID, EventID, Status, CheckinCode string
	CheckIn                          *CheckinSummary
	WaitlistPosition                 *int64
	CreatedAt                        time.Time
	Event                            EventSummary
}

// RegistrationDetail is the API representation, including the signed QR token.
type RegistrationDetail struct {
	ID               string          `json:"id"`
	EventID          string          `json:"eventId"`
	Status           string          `json:"status"`
	QRToken          *string         `json:"qrToken"`
	CheckinCode      *string         `json:"checkinCode"`
	CheckIn          *CheckinSummary `json:"checkIn"`
	WaitlistPosition *int64          `json:"waitlistPosition"`
	CreatedAt        time.Time       `json:"createdAt"`
	Event            EventSummary    `json:"event"`
}

// RegistrationReader loads a registration only when the student owns it
// (otherwise ErrNotFound).
type RegistrationReader interface {
	OwnedRegistration(ctx context.Context, student, registration string) (RegistrationView, error)
}

type DetailService struct {
	reader RegistrationReader
	qr     QRCodec
}

func NewDetailService(reader RegistrationReader, qr QRCodec) *DetailService {
	return &DetailService{reader: reader, qr: qr}
}

func (s *DetailService) Detail(ctx context.Context, student, registration string) (RegistrationDetail, error) {
	v, err := s.reader.OwnedRegistration(ctx, student, registration)
	if err != nil {
		return RegistrationDetail{}, err
	}
	d := RegistrationDetail{ID: v.ID, EventID: v.EventID, Status: v.Status, CheckIn: v.CheckIn,
		WaitlistPosition: v.WaitlistPosition, CreatedAt: v.CreatedAt, Event: v.Event}
	if v.Status == "REGISTERED" {
		token := s.qr.Sign(v.ID, v.EventID)
		code := v.CheckinCode
		if len(code) == 8 {
			code = code[:4] + "-" + code[4:]
		}
		d.QRToken, d.CheckinCode = &token, &code
	}
	return d, nil
}

// CheckinService resolves signed QR tokens before delegating to the store.
type CheckinService struct {
	store  CheckinStore
	qr     QRCodec
	policy CheckinPolicy
}

func NewCheckinService(store CheckinStore, qr QRCodec) *CheckinService {
	return &CheckinService{store: store, qr: qr}
}

func (s *CheckinService) CheckIn(ctx context.Context, actor, event string, in CheckinInput) (CheckinResult, error) {
	if err := s.policy.ValidateInput(in); err != nil {
		return CheckinResult{}, err
	}
	if in.Method == "QR" {
		registration, err := s.qr.Verify(in.QRToken, event)
		if err != nil {
			return CheckinResult{}, ErrInvalidQR
		}
		in = CheckinInput{Method: "QR", RegistrationID: registration}
	}
	return s.store.CheckIn(ctx, actor, event, in)
}

func (s *CheckinService) UndoCheckIn(ctx context.Context, actor, registration string) error {
	return s.store.UndoCheckIn(ctx, actor, registration)
}
