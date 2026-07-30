package dashboard

import (
	"context"
	"time"

	"github.com/esrid/garage/internal/core/appointment"
	"github.com/esrid/garage/internal/web/views"
)

// AppointmentTodayProvider maps the planning domain into F04's frozen view DTO.
// Calls and tasks stay empty until their backend features exist.
type AppointmentTodayProvider struct {
	reader appointment.DayReader
}

func NewAppointmentTodayProvider(reader appointment.DayReader) *AppointmentTodayProvider {
	return &AppointmentTodayProvider{reader: reader}
}

func (p *AppointmentTodayProvider) Today(ctx context.Context, day time.Time) (views.Today, error) {
	planningDay, err := p.reader.Day(ctx, day)
	if err != nil {
		return views.Today{}, err
	}
	result := views.Today{
		Day:          planningDay.Date,
		Calls:        make([]views.Call, 0),
		Appointments: make([]views.Appointment, 0, len(planningDay.Appointments)),
		Tasks:        make([]views.Task, 0),
	}
	location := planningDay.Date.Location()
	for _, entry := range planningDay.Appointments {
		result.Appointments = append(result.Appointments, views.Appointment{
			ID:           entry.ID,
			Start:        entry.Start.In(location),
			End:          entry.End.In(location),
			CustomerName: entry.CustomerName,
			Vehicle:      entry.VehicleLabel,
			Plate:        entry.Plate,
			Service:      entry.ServiceLabel,
			Status:       string(entry.Status),
		})
	}
	return result, nil
}
