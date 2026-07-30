package usage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/esrid/garage/internal/core/conversation"
	"github.com/esrid/garage/internal/core/domain"
)

var martinique = time.FixedZone("AST", -4*60*60)

type usageStub struct {
	usage  conversation.Usage
	err    error
	months []time.Time
}

func (s *usageStub) Usage(_ context.Context, month time.Time) (conversation.Usage, error) {
	s.months = append(s.months, month)
	if s.err != nil {
		return conversation.Usage{}, s.err
	}
	local := month.In(martinique)
	result := s.usage
	result.Month = time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, martinique)
	result.Timezone = "America/Martinique"
	return result, nil
}

func get(t *testing.T, reader conversation.UsageReader, target string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewHandler(reader)
	h.now = func() time.Time { return time.Date(2026, 7, 30, 9, 0, 0, 0, martinique) }
	response := httptest.NewRecorder()
	h.Page(response, httptest.NewRequest(http.MethodGet, target, nil))
	return response
}

func TestUsageShowsMinutesAgainstTheQuota(t *testing.T) {
	stub := &usageStub{usage: conversation.Usage{Calls: 42, Seconds: 9_030, QuotaMinutes: 750}}

	response := get(t, stub, "/app/usage")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	// 9030 seconds is 150.5 minutes, rounded up per the billing rule.
	for _, want := range []string{"juillet 2026", "151 min", "750 min", "599 min", "42", "20 %"} {
		if !strings.Contains(body, want) {
			t.Errorf("page is missing %q", want)
		}
	}
	// Below 70 % nothing shouts: an alert that fires at 20 % is one people learn
	// to ignore.
	if strings.Contains(body, "du quota mensuel consommé") {
		t.Error("the page raised an alert below the first threshold")
	}
}

func TestUsageAlertsAtEachThreshold(t *testing.T) {
	for name, test := range map[string]struct {
		minutes int
		want    string
	}{
		"under":   {500, ""},
		"seventy": {530, "70 % du quota mensuel consommé"},
		"eighty":  {650, "86 % du quota mensuel consommé"},
		"over":    {800, "Quota mensuel atteint"},
	} {
		t.Run(name, func(t *testing.T) {
			stub := &usageStub{usage: conversation.Usage{Seconds: test.minutes * 60, QuotaMinutes: 750}}
			body := get(t, stub, "/app/usage").Body.String()
			if test.want == "" {
				if strings.Contains(body, "quota mensuel consommé") || strings.Contains(body, "Quota mensuel atteint") {
					t.Error("an alert fired below the first threshold")
				}
				return
			}
			if !strings.Contains(body, test.want) {
				t.Errorf("page does not show %q", test.want)
			}
		})
	}
}

// Going over must be visible as a number even though the bar cannot be longer
// than full.
func TestUsageOverQuotaKeepsTheRealNumber(t *testing.T) {
	stub := &usageStub{usage: conversation.Usage{Seconds: 975 * 60, QuotaMinutes: 750}}

	body := get(t, stub, "/app/usage").Body.String()
	if !strings.Contains(body, "130 %") {
		t.Error("the page hides the overage behind a full bar")
	}
	if !strings.Contains(body, "0 min") {
		t.Error("the page does not show that nothing remains")
	}
}

// A month is a civil date: it only means something in the workshop's timezone.
func TestUsageMonthParameterIsReadInTheWorkshopTimezone(t *testing.T) {
	stub := &usageStub{usage: conversation.Usage{QuotaMinutes: 750}}
	get(t, stub, "/app/usage?month=2026-05")

	if len(stub.months) != 2 {
		t.Fatalf("Usage called %d times, want 2", len(stub.months))
	}
	if got := stub.months[1].In(martinique).Format("2006-01"); got != "2026-05" {
		t.Errorf("asked for %s, want 2026-05", got)
	}
}

func TestUsageKeepsRenderingOnAnUnreadableMonth(t *testing.T) {
	stub := &usageStub{usage: conversation.Usage{QuotaMinutes: 750}}

	body := get(t, stub, "/app/usage?month=mai").Body.String()
	if !strings.Contains(body, "Mois illisible") {
		t.Error("the page does not say the month was unreadable")
	}
	if !strings.Contains(body, "juillet 2026") {
		t.Error("the page should fall back to the current month")
	}
}

func TestUsageDegradesWithoutLeakingTheError(t *testing.T) {
	stub := &usageStub{err: errors.New("database is down")}

	response := get(t, stub, "/app/usage")
	if response.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "momentanément indisponible") || strings.Contains(body, "database is down") {
		t.Errorf("degraded page is wrong: %.200s", body)
	}

	unauthorized := &usageStub{err: &domain.UnauthorizedError{Message: "tenant context required"}}
	if code := get(t, unauthorized, "/app/usage").Code; code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
}
