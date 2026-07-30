package customer

import (
	"strings"
	"unicode"

	"github.com/esrid/garage/internal/core/domain"
)

func NormalizePhone(phone string) (string, error) {
	var normalized strings.Builder
	for _, character := range strings.TrimSpace(phone) {
		switch {
		case unicode.IsSpace(character):
			continue
		case character == '.' || character == '-' || character == '(' || character == ')':
			continue
		default:
			normalized.WriteRune(character)
		}
	}

	value := normalized.String()
	if strings.HasPrefix(value, "00") {
		value = "+" + value[2:]
	}
	if len(value) < 9 || len(value) > 16 || value[0] != '+' {
		return "", invalidPhoneError()
	}
	for _, character := range value[1:] {
		if character < '0' || character > '9' {
			return "", invalidPhoneError()
		}
	}
	return value, nil
}

func invalidPhoneError() error {
	return &domain.ValidationError{
		Entity: "customer",
		Errors: map[string]string{"phone": "must be an international number with 8 to 15 digits"},
	}
}
