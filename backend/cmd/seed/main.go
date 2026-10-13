package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/meddesk/meddesk/backend/internal/seed"
)

func main() { os.Exit(run()) }

func run() int {
	accounts, err := seed.Accounts(os.Getenv("APP_ENV"), os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	if os.Getenv("DATABASE_URL") == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not configure seed database")
		return 1
	}
	defer pool.Close()
	created, err := seed.Run(ctx, pool, os.Getenv("APP_ENV"), accounts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "development seed failed; database and credentials were not logged")
		return 1
	}
	fmt.Printf("Development accounts: created %d; existing accounts and clinic settings preserved. Passwords remain in private environment configuration.\n", created)
	return 0
}
