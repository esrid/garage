package domain

import (
	"strings"
	"unicode"
)

// NormalizePhone turns what a human dictated into E.164.
//
// It lives in the domain rather than in one entity's package because three of
// them need the same rule - a customer's number, a follow-up's number, and the
// number a workshop hands calls to - and tenant cannot import customer without a
// cycle. customer.NormalizePhone stays as the name F01 froze, delegating here.
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
	return &ValidationError{Entity: "phone", Errors: map[string]string{"phone": "must be a reachable phone number in international format"}}
}
