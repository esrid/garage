package handlers

import (
	"context"
	"time"

	"github.com/esrid/garage/internal/web/views"
)

// FixtureToday stands in for Agent A's real provider until F02A exists.
//
// It is presentation fixture data: no business rules, no persistence, no
// tenant lookup. Times are derived from the requested day so the page always
// looks like "today" without reading a clock of its own.
//
// TODO(F02A): delete this file and inject the real provider from the DI root.
type FixtureToday struct{}

func (FixtureToday) Today(_ context.Context, day time.Time) (views.Today, error) {
	at := func(hour, minute int) time.Time {
		return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, day.Location())
	}

	return views.Today{
		Day: day,
		Calls: []views.Call{
			{
				ID: "call-1", At: at(8, 12), Duration: 4*time.Minute + 20*time.Second,
				CustomerName: "Marie Lubin", Phone: "0596000001",
				Subject: "Vidange + révision", Outcome: "booked",
			},
			{
				ID: "call-2", At: at(9, 3), Duration: 2*time.Minute + 5*time.Second,
				Phone: "0696000002", Subject: "Devis embrayage", Outcome: "quote",
			},
			{
				ID: "call-3", At: at(10, 41), Duration: time.Minute + 12*time.Second,
				CustomerName: "Jean-Claude Sainte-Rose", Phone: "0596000003",
				Subject: "Demande hors périmètre", Outcome: "transferred", Transferred: true,
			},
		},
		Appointments: []views.Appointment{
			{
				ID: "rdv-1", Start: at(9, 30), End: at(10, 30),
				CustomerName: "Marie Lubin", Vehicle: "Clio IV", Plate: "AB-123-CD",
				Service: "Vidange", Status: "confirmed",
			},
			{
				ID: "rdv-2", Start: at(14, 0), End: at(15, 0),
				CustomerName: "Garage Morne-Rouge", Vehicle: "Hilux",
				Service: "Diagnostic freinage", Status: "pending",
			},
		},
		Tasks: []views.Task{
			{
				ID: "task-1", CreatedAt: at(9, 5), Kind: "quote",
				Phone: "0696000002", Note: "Rappeler pour le devis embrayage",
			},
		},
	}, nil
}
