package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/meddesk/meddesk/backend/internal/bootstrap"
	"github.com/meddesk/meddesk/backend/internal/config"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
	"github.com/meddesk/meddesk/backend/internal/platform/health"
	"github.com/meddesk/meddesk/backend/internal/platform/postgres"
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func readPassword(input io.Reader) (string, error) {
	line, err := bufio.NewReader(io.LimitReader(input, 74)).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", bootstrap.ErrInvalidInput
	}
	password := strings.TrimSuffix(line, "\n")
	if len(password) < 12 || len(password) > 72 {
		return "", bootstrap.ErrInvalidInput
	}
	return password, nil
}

func run(args []string, input io.Reader, output, errorOutput io.Writer) int {
	flags := flag.NewFlagSet("bootstrap-admin", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	confirm := flags.Bool("confirm-empty-database", false, "create the first administrator only when no users exist")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	if !*confirm || flags.NArg() != 0 {
		fmt.Fprintln(errorOutput, "explicit -confirm-empty-database is required; password must arrive on stdin")
		return 1
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(errorOutput, "bootstrap configuration invalid")
		return 1
	}
	if cfg.Environment != "production" {
		fmt.Fprintln(errorOutput, "bootstrap-admin requires APP_ENV=production; development uses its separate seed")
		return 1
	}
	password, err := readPassword(input)
	if err != nil {
		fmt.Fprintln(errorOutput, err.Error())
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := postgres.NewWithRootCA(ctx, cfg.DatabaseURL, cfg.RequestTimeout, cfg.DatabaseCACert)
	if err != nil {
		fmt.Fprintln(errorOutput, "bootstrap database configuration invalid")
		return 1
	}
	defer pool.Close()
	if err := health.NewService(pool, db.New(pool)).Ready(ctx); err != nil {
		fmt.Fprintln(errorOutput, "bootstrap requires a reachable database with the current clean migrations")
		return 1
	}
	name := os.Getenv("BOOTSTRAP_ADMIN_NAME")
	if name == "" {
		name = "Әкімші"
	}
	department := os.Getenv("BOOTSTRAP_ADMIN_DEPARTMENT")
	if department == "" {
		department = "Әкімшілік"
	}
	err = bootstrap.Run(ctx, pool, bootstrap.Input{
		Email: os.Getenv("BOOTSTRAP_ADMIN_EMAIL"), DisplayName: name, Department: department, Password: password,
	})
	if err != nil {
		fmt.Fprintln(errorOutput, err.Error())
		return 1
	}
	fmt.Fprintln(output, "Initial administrator created. Sign in using the supplied email and password.")
	return 0
}
