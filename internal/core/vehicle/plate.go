package vehicle

import (
	"strings"

	"github.com/esrid/garage/internal/core/domain"
)

func NormalizePlate(plate string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(plate))
	var normalized strings.Builder
	for _, character := range value {
		if character == ' ' || character == '-' {
			continue
		}
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return "", invalidPlateError()
		}
		normalized.WriteRune(character)
	}
	value = normalized.String()
	if len(value) < 2 || len(value) > 15 {
		return "", invalidPlateError()
	}
	return value, nil
}

func invalidPlateError() error {
	return &domain.ValidationError{
		Entity: "vehicle",
		Errors: map[string]string{"plate": "must contain 2 to 15 letters or digits"},
	}
}
