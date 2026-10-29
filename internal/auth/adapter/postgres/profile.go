package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/auth/application"
	"github.com/qldapm-hcmut/ticket-system-uni/internal/db"
)

type ProfileReader struct{ queries *db.Queries }

func NewProfileReader(pool *pgxpool.Pool) *ProfileReader { return &ProfileReader{db.New(pool)} }
func (r *ProfileReader) ReadProfile(ctx context.Context, id string) (application.Profile, error) {
	uid, err := uuid(id)
	if err != nil {
		return application.Profile{}, application.ErrUnauthenticated
	}
	row, err := r.queries.GetUserByID(ctx, uid)
	if err != nil {
		return application.Profile{}, mapError(err)
	}
	if !row.IsActive {
		return application.Profile{}, application.ErrUnauthenticated
	}
	u, err := user(row, nil)
	if err != nil {
		return application.Profile{}, err
	}
	memberships, err := r.queries.ListUserMemberships(ctx, uid)
	if err != nil {
		return application.Profile{}, mapError(err)
	}
	profile := application.Profile{User: u, Memberships: []application.Membership{}}
	for _, m := range memberships {
		profile.Memberships = append(profile.Memberships, application.Membership{UnitID: m.UnitID.String(), Code: m.Code, Name: m.Name, MemberRole: string(m.MemberRole)})
	}
	return profile, nil
}
