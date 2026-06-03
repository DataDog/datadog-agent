// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/db/setup"
)

// desiredSettings lists all server settings the command needs to configure,
// in the order they should appear in the plan.
var desiredSettings = []struct {
	name            string
	desired         string
	requiresRestart bool
}{
	{"shared_preload_libraries", "pg_stat_statements", true},
	{"track_activity_query_size", "4096", true},
	{"pg_stat_statements.max", "10000", true},
	{"pg_stat_statements.track", "all", false},
	{"track_io_timing", "on", false},
	{"pg_stat_statements.track_utility", "on", false},
}

// awsParamGroupInstructions returns the RDS/Aurora remediation for a setting.
func awsParamGroupInstructions(name, value string) string {
	return fmt.Sprintf("Set %s = '%s'\n    → RDS Console → Parameter Groups → your group → save → reboot instance", name, value)
}

// cloudSQLFlagInstructions returns the Cloud SQL remediation for a setting.
func cloudSQLFlagInstructions(name, value string) string {
	return fmt.Sprintf("Set %s = '%s'\n    → Cloud SQL Console → your instance → Edit → Database flags → save → restart instance", name, value)
}

// Plan maps a SetupState to an ordered list of Operations. No SQL is executed.
func Plan(ctx context.Context, conn *pgx.Conn, state *setup.SetupState, cfg setup.Config) ([]*setup.Operation, error) {
	if err := checkPrivileges(ctx, conn, state, cfg); err != nil {
		return nil, err
	}

	var ops []*setup.Operation

	// --- User operations ---
	ops = append(ops, planUserOps(state, cfg)...)

	// --- Cluster-level grant ---
	ops = append(ops, planGrantOps(state, cfg)...)

	// --- Server-level settings ---
	ops = append(ops, planSettingOps(state)...)

	// --- Per-database operations ---
	for _, db := range state.Databases {
		ops = append(ops, planPerDBOps(state, cfg, db)...)
	}

	return ops, nil
}

func checkPrivileges(ctx context.Context, conn *pgx.Conn, state *setup.SetupState, cfg setup.Config) error {
	if state.UserExists {
		return nil // No CREATE USER needed; privilege check not required.
	}
	var rolsuper, rolcreaterole bool
	row := conn.QueryRow(ctx, sqlCheckPrivileges)
	if err := row.Scan(&rolsuper, &rolcreaterole); err != nil {
		return fmt.Errorf("checking connection privileges: %w", err)
	}
	if !rolsuper && !rolcreaterole {
		return fmt.Errorf("connected user lacks superuser or createrole — cannot create user %q", cfg.DatadogUser)
	}
	return nil
}

func planUserOps(state *setup.SetupState, cfg setup.Config) []*setup.Operation {
	if !state.UserExists {
		return []*setup.Operation{{
			Kind:        setup.KindSQL,
			Description: fmt.Sprintf("create user %q", cfg.DatadogUser),
			SQL:         sqlCreateUser,
			Args:        []any{cfg.DatadogUser, cfg.DatadogPassword},
			RedactSQL:   true,
		}}
	}

	if cfg.DatadogPassword != "" {
		return []*setup.Operation{{
			Kind:        setup.KindSQL,
			Description: fmt.Sprintf("sync password for user %q", cfg.DatadogUser),
			SQL:         sqlAlterUserPassword,
			Args:        []any{cfg.DatadogUser, cfg.DatadogPassword},
			RedactSQL:   true,
		}}
	}

	return []*setup.Operation{{
		Kind:        setup.KindSkip,
		Description: fmt.Sprintf("user %q — already exists", cfg.DatadogUser),
		Status:      setup.StatusSkipped,
	}}
}

func planGrantOps(state *setup.SetupState, cfg setup.Config) []*setup.Operation {
	var ops []*setup.Operation

	if state.PGVersion >= 10 {
		ops = append(ops, &setup.Operation{
			Kind:        setup.KindSQL,
			Description: fmt.Sprintf("GRANT pg_monitor TO %q", cfg.DatadogUser),
			SQL:         sqlGrantPGMonitor,
			Args:        []any{cfg.DatadogUser},
		})
	} else {
		ops = append(ops, &setup.Operation{
			Kind:        setup.KindSQL,
			Description: fmt.Sprintf("GRANT pg_stat_* tables to %q (PG 9.6)", cfg.DatadogUser),
			SQL:         sqlGrantPG96,
			Args:        []any{cfg.DatadogUser},
		})
	}

	// Aurora/RDS PG 15 requires INHERIT on the datadog role.
	if state.PGVersion >= 15 && (state.Flavor == setup.FlavorRDS || state.Flavor == setup.FlavorAurora) {
		ops = append(ops, &setup.Operation{
			Kind:        setup.KindSQL,
			Description: fmt.Sprintf("ALTER ROLE %q INHERIT (RDS/Aurora PG 15+)", cfg.DatadogUser),
			SQL:         sqlAlterRoleInherit,
			Args:        []any{cfg.DatadogUser},
		})
	}

	return ops
}

func planSettingOps(state *setup.SetupState) []*setup.Operation {
	var ops []*setup.Operation

	for _, s := range desiredSettings {
		current := state.CurrentSettings[s.name]

		switch state.Flavor {
		case setup.FlavorSelfHosted:
			ops = append(ops, planSelfHostedSetting(state, s.name, s.desired, current, s.requiresRestart)...)

		case setup.FlavorRDS, setup.FlavorAurora:
			ops = append(ops, planAWSSetting(state, s.name, s.desired, current)...)

		case setup.FlavorCloudSQL:
			ops = append(ops, planCloudSQLSetting(s.name, s.desired, current)...)
		}
	}

	return ops
}

func planSelfHostedSetting(state *setup.SetupState, name, desired, current string, requiresRestart bool) []*setup.Operation {
	if name == "shared_preload_libraries" {
		return planSPL(state, desired, current, requiresRestart)
	}

	if current == desired {
		return []*setup.Operation{{
			Kind:        setup.KindSkip,
			Description: fmt.Sprintf("%s = %s — already set", name, desired),
			SettingName: name,
			Status:      setup.StatusSkipped,
		}}
	}

	// Guard: skip if a restart for this setting is already pending.
	for _, pr := range state.PendingRestart {
		if pr == name {
			return []*setup.Operation{{
				Kind:        setup.KindSkip,
				Description: fmt.Sprintf("%s — restart already pending, skipping ALTER SYSTEM", name),
				SettingName: name,
				Status:      setup.StatusSkipped,
			}}
		}
	}

	kind := setup.KindAlterSys
	if !requiresRestart {
		kind = setup.KindReload
	}
	return []*setup.Operation{{
		Kind:            kind,
		Description:     fmt.Sprintf("ALTER SYSTEM SET %s = '%s'", name, desired),
		SQL:             fmt.Sprintf("ALTER SYSTEM SET %s = '%s'", name, desired),
		SettingName:     name,
		RequiresRestart: requiresRestart,
	}}
}

// planSPL handles shared_preload_libraries with read-then-append semantics.
func planSPL(state *setup.SetupState, desired, current string, requiresRestart bool) []*setup.Operation {
	libs := strings.Split(current, ",")
	for _, lib := range libs {
		if strings.TrimSpace(lib) == desired {
			return []*setup.Operation{{
				Kind:        setup.KindSkip,
				Description: fmt.Sprintf("shared_preload_libraries already contains '%s'", desired),
				SettingName: "shared_preload_libraries",
				Status:      setup.StatusSkipped,
			}}
		}
	}

	// Guard: pending restart already in flight.
	for _, pr := range state.PendingRestart {
		if pr == "shared_preload_libraries" {
			return []*setup.Operation{{
				Kind:        setup.KindSkip,
				Description: "shared_preload_libraries — restart already pending; check postgresql.auto.conf before restarting",
				SettingName: "shared_preload_libraries",
				Status:      setup.StatusSkipped,
			}}
		}
	}

	newValue := desired
	if current != "" {
		newValue = current + "," + desired
	}
	return []*setup.Operation{{
		Kind:            setup.KindAlterSys,
		Description:     fmt.Sprintf("ALTER SYSTEM SET shared_preload_libraries = '%s'", newValue),
		SQL:             fmt.Sprintf("ALTER SYSTEM SET shared_preload_libraries = '%s'", newValue),
		SettingName:     "shared_preload_libraries",
		RequiresRestart: requiresRestart,
	}}
}

func planAWSSetting(state *setup.SetupState, name, desired, current string) []*setup.Operation {
	// Aurora: shared_preload_libraries includes pg_stat_statements by default.
	if name == "shared_preload_libraries" && state.Flavor == setup.FlavorAurora {
		if strings.Contains(current, desired) {
			return []*setup.Operation{{
				Kind:        setup.KindSkip,
				Description: fmt.Sprintf("shared_preload_libraries already contains '%s' on Aurora — skipped", desired),
				SettingName: name,
				Status:      setup.StatusSkipped,
			}}
		}
	}

	return []*setup.Operation{{
		Kind:              setup.KindManualStep,
		Description:       fmt.Sprintf("[AWS Parameter Group] %s = '%s'", name, desired),
		SettingName:       name,
		ManualInstruction: awsParamGroupInstructions(name, desired),
		Status:            setup.StatusManual,
	}}
}

func planCloudSQLSetting(name, desired, current string) []*setup.Operation {
	// Cloud SQL pre-loads pg_stat_statements.
	if name == "shared_preload_libraries" {
		return []*setup.Operation{{
			Kind:        setup.KindSkip,
			Description: "shared_preload_libraries — pre-loaded on Cloud SQL",
			SettingName: name,
			Status:      setup.StatusSkipped,
		}}
	}

	if current == desired {
		return []*setup.Operation{{
			Kind:        setup.KindSkip,
			Description: fmt.Sprintf("%s = %s — already set", name, desired),
			SettingName: name,
			Status:      setup.StatusSkipped,
		}}
	}

	return []*setup.Operation{{
		Kind:              setup.KindManualStep,
		Description:       fmt.Sprintf("[Cloud SQL Database Flag] %s = '%s'", name, desired),
		SettingName:       name,
		ManualInstruction: cloudSQLFlagInstructions(name, desired),
		Status:            setup.StatusManual,
	}}
}

func planPerDBOps(state *setup.SetupState, cfg setup.Config, db string) []*setup.Operation {
	return []*setup.Operation{
		{
			Kind:        setup.KindSQL,
			Description: "CREATE EXTENSION IF NOT EXISTS pg_stat_statements",
			SQL:         sqlCreateExtension,
			Database:    db,
		},
		{
			Kind:        setup.KindSQL,
			Description: "CREATE SCHEMA IF NOT EXISTS datadog",
			SQL:         sqlCreateSchema,
			Database:    db,
		},
		{
			Kind:        setup.KindSQL,
			Description: fmt.Sprintf("GRANT USAGE ON SCHEMA datadog TO %q", cfg.DatadogUser),
			SQL:         sqlGrantSchemaUsage,
			Args:        []any{cfg.DatadogUser},
			Database:    db,
		},
		{
			Kind:        setup.KindSQL,
			Description: "CREATE OR REPLACE FUNCTION datadog.pg_stat_activity()",
			SQL:         sqlFuncPgStatActivity,
			Database:    db,
		},
		{
			Kind:        setup.KindSQL,
			Description: "CREATE OR REPLACE FUNCTION datadog.pg_stat_statements()",
			SQL:         sqlFuncPgStatStatements,
			Database:    db,
		},
		{
			Kind:        setup.KindSQL,
			Description: "CREATE OR REPLACE FUNCTION datadog.explain_statement()",
			SQL:         sqlFuncExplainStatement,
			Database:    db,
		},
	}
}
