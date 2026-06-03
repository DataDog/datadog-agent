// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package postgres implements 'agent db setup postgres'.
package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/db/setup"
	pgsetup "github.com/DataDog/datadog-agent/pkg/cli/subcommands/db/setup/postgres"
)

type cliParams struct {
	datadogUser     string
	datadogPassword string
	databases       []string
	allDatabases    bool
	dryRun          bool
	output          string
}

// Commands returns the 'db setup postgres' subcommand.
func Commands() []*cobra.Command {
	params := &cliParams{}

	cmd := &cobra.Command{
		Use:   "postgres <connection-uri>",
		Short: "Configure a Postgres instance for DBM",
		Long: `Idempotently configures a Postgres instance for Datadog Database Monitoring.

Handles the "Configure Postgres settings" and "Grant agent access" sections
of the DBM setup docs for self-hosted Postgres, RDS, Aurora, and Cloud SQL.`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validateParams(params, args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), params, args[0])
		},
	}

	cmd.Flags().StringVar(&params.datadogUser, "datadog-user", "datadog", "Agent DB user to create/validate")
	cmd.Flags().StringVar(&params.datadogPassword, "datadog-password", "", "Password for the agent user (env: DD_DBM_DATADOG_PASSWORD)")
	cmd.Flags().StringSliceVar(&params.databases, "databases", nil, "Databases to configure (default: database in URI)")
	cmd.Flags().BoolVar(&params.allDatabases, "all-databases", false, "Configure all non-template databases")
	cmd.Flags().BoolVar(&params.dryRun, "dry-run", false, "Print plan, apply nothing")
	cmd.Flags().StringVar(&params.output, "output", "text", "Output format: text or json")

	return []*cobra.Command{cmd}
}

// validateParams performs pre-flight checks before any DB connection is opened.
func validateParams(params *cliParams, uri string) error {
	if params.datadogPassword == "" {
		if v := os.Getenv("DD_DBM_DATADOG_PASSWORD"); v != "" {
			params.datadogPassword = v
		}
	}

	// --all-databases and --databases are mutually exclusive.
	if params.allDatabases && len(params.databases) > 0 {
		return fmt.Errorf("--all-databases and --databases are mutually exclusive")
	}

	// Password is required when creating the user for the first time, but we
	// don't know yet whether the user exists. We defer the actual check to
	// after Detect. Here we only check obvious issues.
	_ = uri // URI format is validated when pgx.ParseConfig is called in run().
	return nil
}

func run(ctx context.Context, params *cliParams, uri string) error {
	connConfig, defaultDB, err := pgsetup.ParseConnURI(uri)
	if err != nil {
		return err
	}

	// Determine which databases to configure.
	databases := params.databases
	if len(databases) == 0 && !params.allDatabases {
		databases = []string{defaultDB}
	}

	cfg := setup.Config{
		DatadogUser:     params.datadogUser,
		DatadogPassword: params.datadogPassword,
		Databases:       databases,
		AllDatabases:    params.allDatabases,
		DryRun:          params.dryRun,
		Output:          params.output,
	}

	conn, err := pgsetup.ConnectWithConfig(ctx, connConfig)
	if err != nil {
		return fmt.Errorf("connecting to Postgres: %w", err)
	}
	defer conn.Close(ctx)

	state, err := pgsetup.Detect(ctx, conn, cfg)
	if err != nil {
		return fmt.Errorf("detect phase: %w", err)
	}

	// Now that we know whether the user exists, validate the password requirement.
	if !state.UserExists && params.datadogPassword == "" {
		return fmt.Errorf("--datadog-password is required when creating user %q for the first time", params.datadogUser)
	}

	ops, err := pgsetup.Plan(ctx, conn, state, cfg)
	if err != nil {
		return fmt.Errorf("plan phase: %w", err)
	}

	var result *setup.SetupResult
	if params.dryRun {
		result = dryRunResult(ops, state)
	} else {
		result = pgsetup.Apply(ctx, conn, connConfig, ops, state)
	}

	if err := setup.Render(os.Stdout, result, params.output); err != nil {
		return err
	}

	if result.Outcome == "failure" || result.ManualSteps || result.RestartNeeded {
		return fmt.Errorf("%s", strings.ToUpper(result.Outcome))
	}
	return nil
}

// dryRunResult marks all non-skip operations as pending and returns a dry-run result.
func dryRunResult(ops []*setup.Operation, state *setup.SetupState) *setup.SetupResult {
	for _, op := range ops {
		if op.Status == setup.StatusSkipped || op.Kind == setup.KindSkip {
			op.Status = setup.StatusSkipped
		} else if op.Kind == setup.KindManualStep {
			op.Status = setup.StatusManual
		} else {
			op.Status = setup.StatusPending
		}
	}

	manualSteps := false
	for _, op := range ops {
		if op.Kind == setup.KindManualStep {
			manualSteps = true
			break
		}
	}

	return &setup.SetupResult{
		Operations:  ops,
		Flavor:      state.Flavor,
		PGVersion:   state.PGVersion,
		ManualSteps: manualSteps,
		Outcome:     "dry_run",
	}
}
