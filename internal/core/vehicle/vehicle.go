package vehicle

import "time"

type Vehicle struct {
	ID              string
	TenantID        string
	CustomerID      string
	Plate           string
	NormalizedPlate string
	Make            string
	Model           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CreateInput struct {
	CustomerID string
	Plate      string
	Make       string
	Model      string
}
