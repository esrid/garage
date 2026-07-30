package ports

import "context"

// ReadinessStore is the persistence capability required by the readiness
// use-case. Database adapters satisfy this without leaking their driver into
// the core.
type ReadinessStore interface {
	Ping(context.Context) error
}
