package customer

import "time"

type Customer struct {
	ID        string
	TenantID  string
	FirstName string
	LastName  string
	Phone     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateInput struct {
	FirstName string
	LastName  string
	Phone     string
}
