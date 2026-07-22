// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package integrations

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/integrations/setup"
)

// setupModule is the integration's setup entry point, run as a module via the
// embedded Python interpreter. The DBM setup logic lives in the Postgres
// integration (integrations-core) so it can reuse the integration's connection
// and database-navigation code; this Go command is only the tunnel to it.
const setupModule = "datadog_checks.postgres.setup"

type setupParams struct {
	datadogUser     string
	datadogPassword string
	updatePassword  bool // must be set explicitly to sync password on re-runs
	databases       []string
	allDatabases    bool
	dryRun          bool
	output          string
	yes             bool
}

// newSetupCommand returns the 'integration setup' subcommand and its children.
// parentParams is the parent integrationCmd's cliParams; useSysPython is read
// from it at run time so the persistent --use-sys-python flag is respected.
func newSetupCommand(_ *command.GlobalParams, parentParams *cliParams) *cobra.Command {
	setupCmd := &cobra.Command{
		Use:   "setup [dbms]",
		Short: "Configure a database instance for Datadog Database Monitoring",
		Long:  `Configure a database instance for Datadog Database Monitoring (DBM).`,
	}

	params := &setupParams{}

	postgresCmd := &cobra.Command{
		Use:   "postgres <connection-uri>",
		Short: "Configure a Postgres instance for DBM",
		Long: `Idempotently configures a Postgres instance for Datadog Database Monitoring.

Handles the "Configure Postgres settings" and "Grant agent access" sections
of the DBM setup docs for self-hosted Postgres, RDS, Aurora, Cloud SQL, and Azure.`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(_ *cobra.Command, _ []string) error {
			if params.datadogPassword == "" {
				if v := os.Getenv("DD_DBM_DATADOG_PASSWORD"); v != "" {
					params.datadogPassword = v
				}
			}
			if params.allDatabases && len(params.databases) > 0 {
				return errors.New("--all-databases and --databases are mutually exclusive")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPostgresSetup(cmd.Context(), params, args[0], parentParams.useSysPython)
		},
	}

	postgresCmd.Flags().StringVar(&params.datadogUser, "datadog-user", "datadog",
		"Agent DB user to create/validate")
	postgresCmd.Flags().StringVar(&params.datadogPassword, "datadog-password", "",
		"Password for the agent user (env: DD_DBM_DATADOG_PASSWORD)")
	postgresCmd.Flags().BoolVar(&params.updatePassword, "update-password", false,
		"Sync --datadog-password to the existing user (skipped by default to avoid breaking live agent connections)")
	postgresCmd.Flags().StringSliceVar(&params.databases, "databases", nil,
		"Databases to configure (default: database in URI)")
	postgresCmd.Flags().BoolVar(&params.allDatabases, "all-databases", false,
		"Configure all non-template databases")
	postgresCmd.Flags().BoolVar(&params.dryRun, "dry-run", false,
		"Print plan, apply nothing")
	postgresCmd.Flags().StringVar(&params.output, "output", "text",
		"Output format: text or json")
	postgresCmd.Flags().BoolVarP(&params.yes, "yes", "y", false,
		"Apply optional settings (e.g. pg_stat_statements.max) without prompting")

	// Suppress cobra's "Error: ..." prefix — we print our own next-steps
	// messages directly so they don't look like unexpected errors.
	postgresCmd.SilenceErrors = true
	postgresCmd.SilenceUsage = true

	setupCmd.AddCommand(postgresCmd)
	return setupCmd
}

func callPython(ctx context.Context, pythonPath, uri string, databases []string, params *setupParams, applyOptionalRestart bool) (*setup.SetupResult, error) {
	argsPayload := map[string]interface{}{
		"connection_uri": uri,
		"config": map[string]interface{}{
			"datadog_user":           params.datadogUser,
			"datadog_password":       params.datadogPassword,
			"update_password":        params.updatePassword,
			"databases":              databases,
			"all_databases":          params.allDatabases,
			"dry_run":                params.dryRun,
			"apply_optional_restart": applyOptionalRestart,
		},
	}
	argsJSON, err := json.Marshal(argsPayload)
	if err != nil {
		return nil, fmt.Errorf("marshaling setup args: %w", err)
	}

	cmd := exec.CommandContext(ctx, pythonPath, "-m", setupModule)
	cmd.Stdin = strings.NewReader(string(argsJSON))
	cmd.Stderr = os.Stderr

	out, runErr := cmd.Output()

	// Always try to parse JSON first — the Python script prints a structured
	// error to stdout even on non-zero exit, which gives a much better message
	// than the raw "exit status 1".
	var pyResult setup.PythonResult
	if jsonErr := json.Unmarshal(out, &pyResult); jsonErr == nil {
		if !pyResult.Success {
			return nil, fmt.Errorf("%s", pyResult.Error)
		}
		return pyResult.Result, nil
	}

	if runErr != nil {
		return nil, fmt.Errorf("setup script failed: %w", runErr)
	}
	return nil, fmt.Errorf("parsing setup output: %s", string(out))
}

func promptOptional(pending []setup.OptionalRestartSetting) bool {
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Optional settings (improve query visibility, not required for DBM to work):")
	for _, s := range pending {
		fmt.Fprintf(os.Stdout, "  • %s = %s  (default: %s)\n", s.Name, s.Desired, defaultOrCurrent(s.Current))
	}
	fmt.Fprintln(os.Stdout, "\nApplying these requires one more PostgreSQL restart.")
	fmt.Fprint(os.Stdout, "Configure them now? [y/N]: ")

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}

func defaultOrCurrent(current string) string {
	if current == "" {
		return "PostgreSQL default"
	}
	return current
}

// defaultDatabaseFromURI extracts the database to configure from the connection
// URI's path component. Parsing the path (rather than splitting on the last '/')
// avoids treating slashes inside query parameters — e.g. ?sslrootcert=/etc/ssl/ca.pem
// or ?host=/var/run/postgresql — as the database name. Falls back to "postgres"
// when the URI has no database path.
func defaultDatabaseFromURI(uri string) string {
	if u, err := url.Parse(uri); err == nil {
		if db := strings.TrimPrefix(u.Path, "/"); db != "" {
			return db
		}
	}
	return "postgres"
}

func runPostgresSetup(ctx context.Context, params *setupParams, uri string, useSysPython bool) error {
	var pythonPath string

	if useSysPython {
		// Use the Python binary on PATH (dev / --use-sys-python mode). The Postgres
		// integration must be importable from it (e.g. `pip install -e`).
		pythonPath = pythonBin
	} else {
		if err := loadPythonInfo(); err != nil {
			return fmt.Errorf("unable to locate agent Python environment: %w", err)
		}
		var err error
		pythonPath, err = getCommandPython(false)
		if err != nil {
			return fmt.Errorf("unable to find embedded Python: %w", err)
		}
	}

	databases := params.databases
	if len(databases) == 0 && !params.allDatabases {
		databases = []string{defaultDatabaseFromURI(uri)}
	}

	result, err := callPython(ctx, pythonPath, uri, databases, params, false)
	if err != nil {
		return err
	}

	if err := setup.Render(os.Stdout, result, params.output); err != nil {
		return err
	}

	if result.Outcome == "failure" {
		return errors.New("setup did not complete — check the output above for details")
	}
	if result.RestartNeeded {
		fmt.Fprintln(os.Stderr, "\nNext steps: restart PostgreSQL, then re-run to apply remaining settings.")
		os.Exit(2)
	}
	if result.ManualSteps {
		fmt.Fprintln(os.Stderr, "\nNext steps: complete the manual steps above, then re-run to verify.")
		os.Exit(2)
	}

	// After a fully successful run, check for deferred optional restart settings.
	if len(result.OptionalRestartPending) > 0 && !params.dryRun {
		applyOptional := params.yes || promptOptional(result.OptionalRestartPending)
		if applyOptional {
			optResult, err := callPython(ctx, pythonPath, uri, databases, params, true)
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout)
			if err := setup.Render(os.Stdout, optResult, params.output); err != nil {
				return err
			}
			if optResult.RestartNeeded {
				fmt.Fprintln(os.Stderr, "\nNext steps: restart PostgreSQL to apply pg_stat_statements.max.")
				os.Exit(2)
			}
		} else {
			fmt.Fprintf(os.Stdout, "\nSkipped. pg_stat_statements.max will use the PostgreSQL default (5000).\n")
			fmt.Fprintf(os.Stdout, "Run with --yes to apply it later (requires a restart).\n")
		}
	}

	return nil
}
