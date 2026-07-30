package settings

import "github.com/esrid/garage/internal/core/tenant"

// View is the settings form plus whatever happened to the last submission.
type View struct {
	Settings tenant.Settings
	Saved    bool
	Notice   string
	Degraded bool
}
