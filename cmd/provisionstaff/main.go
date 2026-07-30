// Command provisionstaff creates the first staff account of a workshop.
//
// F09 deliberately ships no public signup, and nothing else calls
// auth.Service.Provision: without this command a fresh deployment has no account
// at all, so nobody can ever sign in and every /app route answers 401 forever.
//
// The password is read from standard input, never from a flag: a flag lands in
// the shell history and in the process list of everyone on the machine.
//
//	printf '%s' 'the-password' | DATABASE_DSN=... go run ./cmd/provisionstaff \
//	    -tenant-name "Garage Untel" -email patron@garage.mq -name "Jean Untel"
//
// Passing -tenant-id instead of -tenant-name adds a member to an existing
// workshop. The command is idempotent in the useful direction: creating an email
// that already exists fails instead of overwriting a password.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/esrid/garage/internal/adapters/stores/postgres"
	coreauth "github.com/esrid/garage/internal/core/auth"
	"github.com/esrid/garage/internal/core/tenant"

	_ "time/tzdata"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "provisionstaff:", err)
		os.Exit(1)
	}
}

func run() error {
	tenantName := flag.String("tenant-name", "", "create this workshop and put the account in it")
	tenantID := flag.String("tenant-id", "", "existing workshop UUID to add the account to")
	email := flag.String("email", "", "sign-in email")
	displayName := flag.String("name", "", "display name, optional")
	role := flag.String("role", coreauth.RoleOwner, "owner or staff")
	flag.Parse()

	if (*tenantName == "") == (*tenantID == "") {
		return errors.New("pass exactly one of -tenant-name or -tenant-id")
	}
	if strings.TrimSpace(*email) == "" {
		return errors.New("-email is required")
	}
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		return errors.New("DATABASE_DSN is required")
	}
	password, err := readPassword()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	workshop := *tenantID
	if *tenantName != "" {
		created, err := tenant.NewService(store).Create(ctx, tenant.CreateInput{Name: *tenantName})
		if err != nil {
			return fmt.Errorf("create workshop: %w", err)
		}
		workshop = created.ID
		fmt.Printf("workshop created: %s\n", workshop)
	}

	// The tenant travels in the context here exactly as it does behind a session:
	// the provisioning service resolves it from there and never takes it as an
	// argument, so this command cannot bypass the boundary the app enforces.
	staff, err := coreauth.NewService(store).Provision(tenant.WithID(ctx, workshop), coreauth.ProvisionInput{
		Email:       *email,
		DisplayName: *displayName,
		Password:    password,
		Role:        *role,
	})
	if err != nil {
		return fmt.Errorf("provision staff: %w", err)
	}
	fmt.Printf("staff created: %s (%s) in workshop %s\n", staff.Email, staff.Role, workshop)
	return nil
}

// readPassword takes the whole of standard input, minus one trailing newline so
// `printf` and `echo` both behave. It never echoes and never logs the value.
func readPassword() (string, error) {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}
	password := strings.TrimRight(string(raw), "\r\n")
	if password == "" {
		return "", errors.New("the password must be piped on standard input")
	}
	return password, nil
}
