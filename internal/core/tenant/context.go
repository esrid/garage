package tenant

import (
	"context"
	"strings"

	"github.com/esrid/garage/internal/core/domain"
)

type contextKey struct{}

func WithID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, contextKey{}, strings.TrimSpace(tenantID))
}

func IDFromContext(ctx context.Context) (string, error) {
	tenantID, _ := ctx.Value(contextKey{}).(string)
	if tenantID == "" {
		return "", &domain.UnauthorizedError{Message: "tenant context required"}
	}
	return tenantID, nil
}
