package customerfiles

import (
	"strings"

	"github.com/esrid/garage/internal/core/customer"
)

// SearchView is the result list plus anything that stopped it from being read.
type SearchView struct {
	Query    string
	Matches  []customer.Match
	Degraded bool
	Notice   string
}

// title names a customer the way every other screen does: their name when known,
// their number otherwise. Nothing is invented to fill the gap.
func title(person customer.Customer) string {
	name := strings.TrimSpace(person.FirstName + " " + person.LastName)
	if name != "" {
		return name
	}
	if person.Phone != "" {
		return person.Phone
	}
	return "Client inconnu"
}

// plateList joins what a customer drives, for the one-line summary of a result.
func plateList(plates []string) string {
	kept := make([]string, 0, len(plates))
	for _, plate := range plates {
		if strings.TrimSpace(plate) != "" {
			kept = append(kept, plate)
		}
	}
	if len(kept) == 0 {
		return "Aucun véhicule enregistré"
	}
	return strings.Join(kept, " · ")
}

func vehicleLabel(v customer.Vehicle) string {
	label := strings.TrimSpace(v.Make + " " + v.Model)
	if label == "" {
		return "Véhicule sans modèle"
	}
	return label
}
