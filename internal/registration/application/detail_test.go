package application

import (
	"context"
	"errors"
	"testing"
)

type fakeReader struct{ view RegistrationView }

func (f fakeReader) OwnedRegistration(context.Context, string, string) (RegistrationView, error) {
	return f.view, nil
}

type fakeCodec struct{}

func (fakeCodec) Sign(r, e string) string { return "signed." + r + "." + e }
func (fakeCodec) Verify(token, event string) (string, error) {
	if token == "good" {
		return "reg-1", nil
	}
	return "", errors.New("bad")
}

type fakeStore struct{ got CheckinInput }

func (f *fakeStore) CheckIn(_ context.Context, _, _ string, in CheckinInput) (CheckinResult, error) {
	f.got = in
	return CheckinResult{}, nil
}
func (f *fakeStore) UndoCheckIn(context.Context, string, string) error { return nil }

func TestDetailService(t *testing.T) {
	for _, tt := range []struct {
		name, status, code, wantCode string
		wantQR                       bool
	}{
		{"registered", "REGISTERED", "ABCD1234", "ABCD-1234", true},
		{"short code does not panic", "REGISTERED", "AB", "AB", true},
		{"waitlisted gets no QR", "WAITLISTED", "ABCD1234", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d, err := NewDetailService(fakeReader{RegistrationView{ID: "r", EventID: "e", Status: tt.status, CheckinCode: tt.code}}, fakeCodec{}).Detail(context.Background(), "s", "r")
			if err != nil {
				t.Fatal(err)
			}
			if (d.QRToken != nil) != tt.wantQR || (tt.wantQR && *d.CheckinCode != tt.wantCode) || (!tt.wantQR && d.CheckinCode != nil) {
				t.Fatalf("%+v", d)
			}
		})
	}
}

func TestCheckinServiceQR(t *testing.T) {
	store := &fakeStore{}
	svc := NewCheckinService(store, fakeCodec{})
	ctx := context.Background()
	if _, err := svc.CheckIn(ctx, "a", "e", CheckinInput{Method: "QR", QRToken: "bad"}); !errors.Is(err, ErrInvalidQR) {
		t.Fatalf("got %v", err)
	}
	if _, err := svc.CheckIn(ctx, "a", "e", CheckinInput{Method: "QR", QRToken: "good", Note: "x"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("got %v", err)
	}
	if _, err := svc.CheckIn(ctx, "a", "e", CheckinInput{Method: "QR", QRToken: "good"}); err != nil {
		t.Fatal(err)
	}
	if store.got.Method != "QR" || store.got.RegistrationID != "reg-1" || store.got.QRToken != "" {
		t.Fatalf("store input %+v", store.got)
	}
}
