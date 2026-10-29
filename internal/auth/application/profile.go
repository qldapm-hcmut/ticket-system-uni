package application

import "context"

type Membership struct{ UnitID, Code, Name, MemberRole string }
type Profile struct {
	User
	Memberships []Membership
}
type ProfileReader interface {
	ReadProfile(context.Context, string) (Profile, error)
}
