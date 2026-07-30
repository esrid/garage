// Package di is the composition root. Database-specific adapters are selected
// here; the core and HTTP adapter depend only on the capabilities they consume.
package di

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/esrid/garage/internal/adapters/httpserver"
	"github.com/esrid/garage/internal/adapters/stores/postgres"
	"github.com/esrid/garage/internal/adapters/voice"
	"github.com/esrid/garage/internal/config"
	"github.com/esrid/garage/internal/core/appointment"
	coreauth "github.com/esrid/garage/internal/core/auth"
	"github.com/esrid/garage/internal/core/conversation"
	"github.com/esrid/garage/internal/core/customer"
	"github.com/esrid/garage/internal/core/followup"
	"github.com/esrid/garage/internal/core/services"
	"github.com/esrid/garage/internal/core/vehicle"
	"github.com/esrid/garage/internal/features/calls"
	"github.com/esrid/garage/internal/features/dashboard"
	"github.com/esrid/garage/internal/features/identity"
	"github.com/esrid/garage/internal/features/planning"
	"github.com/esrid/garage/internal/features/postcall"
	"github.com/esrid/garage/internal/features/usage"
	"github.com/esrid/garage/internal/features/voicetools"
)

type App struct {
	server          *http.Server
	database        io.Closer
	shutdownTimeout time.Duration
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	voiceAuthenticator, err := voice.NewTokenAuthenticator(cfg.VoiceToolTokens)
	if err != nil {
		return nil, err
	}

	database, err := postgres.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		return nil, err
	}

	readiness := services.NewReadiness(database)
	scheduling := appointment.NewService(database, database, database, database)
	followUpReads := followup.NewReadService(database)
	callHistoryProvider := calls.NewCallHistoryProvider(conversation.NewHistoryService(database), followUpReads)
	// The day view is composed one domain at a time: appointments, then the calls
	// F14 persisted, then the follow-ups F08 recorded.
	dashboardProvider := dashboard.NewTodayWithFollowUpsProvider(
		dashboard.NewTodayWithCallsProvider(dashboard.NewAppointmentTodayProvider(scheduling), callHistoryProvider),
		followUpReads,
	)
	dashboardHandler := dashboard.NewDashboard(dashboardProvider)
	callsHandler := calls.NewCalls(callHistoryProvider)
	usageHandler := usage.NewHandler(conversation.NewUsageService(database))
	planningHandler := planning.NewHandler(scheduling)
	appointmentMutations := planning.NewAppointmentMutations(scheduling)
	openingMutations := planning.NewOpeningMutations(scheduling)
	customerService := customer.NewService(database)
	customerLookup := voicetools.NewCustomerLookup(customerService, voiceAuthenticator)
	customerRecord := voicetools.NewCustomerRecord(customerService, vehicle.NewService(database), voiceAuthenticator)
	appointmentTools := voicetools.NewAppointmentTools(scheduling, voiceAuthenticator)
	followUpTool := voicetools.NewFollowUpTool(followup.NewService(database), voiceAuthenticator)
	postCallWebhook, err := postcall.NewPostCallWebhook(
		conversation.NewService(database),
		cfg.ElevenLabsWebhookSecret,
		cfg.ElevenLabsAgentTenants,
	)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	authenticationService := coreauth.NewService(database)
	authentication := identity.NewAuthentication(authenticationService)
	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpserver.New(httpserver.Deps{
			Readiness:        readiness,
			Sessions:         authenticationService,
			Authentication:   authentication,
			Dashboard:        dashboardHandler,
			Calls:            callsHandler,
			Usage:            usageHandler,
			Planning:         planningHandler,
			Appointments:     appointmentMutations,
			Openings:         openingMutations,
			CustomerLookup:   customerLookup,
			CustomerRecord:   customerRecord,
			AppointmentTools: appointmentTools,
			FollowUpTool:     followUpTool,
			PostCallWebhook:  postCallWebhook,
		}),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
	}

	return &App{
		server:          server,
		database:        database,
		shutdownTimeout: cfg.ShutdownTimeout,
	}, nil
}

func Run(ctx context.Context, cfg config.Config) error {
	app, err := New(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := app.Close(); err != nil {
			slog.Error("close application", "err", err)
		}
	}()
	return app.Run(ctx)
}

func (a *App) Run(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}

	serverResult := make(chan error, 1)
	go func() {
		err := a.server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverResult <- err
	}()
	slog.Info("http server started", "addr", a.server.Addr)

	select {
	case err := <-serverResult:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		_ = a.server.Close()
		return fmt.Errorf("http server shutdown: %w", err)
	}
	if err := <-serverResult; err != nil {
		return fmt.Errorf("http server: %w", err)
	}
	slog.Info("http server stopped")
	return nil
}

func (a *App) Close() error {
	return a.database.Close()
}
