// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package postgres implements the 'agent db setup postgres' subcommand, which
// configures a Postgres instance for Datadog Database Monitoring.
//
// This is a skeleton: it wires up the command interface from the DBM
// Agent-Distributed Setup CLI RFC and validates the connection URI using pgx.
// The Detect/Plan/Apply logic is intentionally not implemented yet.
package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"
)

// cliParams holds the flags and arguments for 'agent db setup postgres'.
type cliParams struct {
	connURI         string
	datadogUser     string
	datadogPassword string
	databases       []string
	allDatabases    bool
	dryRun          bool
	output          string
}

// Command returns the 'postgres' subcommand for 'agent db setup'.
func Command() *cobra.Command {
	params := &cliParams{}

	cmd := &cobra.Command{
		Use:   "postgres <connection-uri>",
		Short: "Configure a Postgres instance for Database Monitoring",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			params.connURI = args[0]
			return run(params)
		},
	}

	cmd.Flags().StringVar(&params.datadogUser, "datadog-user", "datadog",
		"Agent DB user to create/validate")
	cmd.Flags().StringVar(&params.datadogPassword, "datadog-password", "",
		"Password for the agent user (env: DD_DBM_DATADOG_PASSWORD)")
	cmd.Flags().StringSliceVar(&params.databases, "databases", nil,
		"Per-DB scope (default: the database in the URI), comma-separated")
	cmd.Flags().BoolVar(&params.allDatabases, "all-databases", false,
		"Configure all non-template databases")
	cmd.Flags().BoolVar(&params.dryRun, "dry-run", false,
		"Print plan, apply nothing")
	cmd.Flags().StringVar(&params.output, "output", "text",
		"text (default) | json")

	return cmd
}

// run is a skeleton implementation. It parses and validates the connection URI
// with pgx, then prints the (not-yet-implemented) plan. The raw URI is never
// logged: only non-secret host/database metadata is surfaced.
func run(params *cliParams) error {
	if params.connURI == "" {
		return errors.New("a Postgres connection URI is required")
	}

	cfg, err := pgx.ParseConfig(params.connURI)
	if err != nil {
		return fmt.Errorf("invalid connection URI: %w", err)
	}

	fmt.Printf("Planned DBM setup for Postgres host=%s database=%s agent-user=%s (dry-run=%t)\n",
		cfg.Host, cfg.Database, pgx.Identifier{params.datadogUser}.Sanitize(), params.dryRun)
	fmt.Println("Detect/Plan/Apply not yet implemented (skeleton).")

	return nil
}
