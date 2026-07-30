package customer

import "github.com/esrid/garage/internal/core/domain"

// NormalizePhone is the name frozen by the F01 contract. The rule itself lives in
// the domain, where the workshop's transfer number and the follow-up requests
// reach it too.
func NormalizePhone(phone string) (string, error) {
	return domain.NormalizePhone(phone)
}
