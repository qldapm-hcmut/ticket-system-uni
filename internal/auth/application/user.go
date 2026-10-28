package application

// User is the account representation used by authentication use cases.
type User struct {
	ID          string
	Email       string
	FullName    string
	GoogleSub   string // Empty only for an unlinked bootstrap account.
	StudentCode string
	IsAdmin     bool
	IsActive    bool
}
