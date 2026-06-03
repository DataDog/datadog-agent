// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package postgres

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/db/setup"
)

// Detect runs read-only queries against the target database and returns a
// SetupState snapshot. Nothing is written in this phase.
func Detect(ctx context.Context, conn *pgx.Conn, cfg setup.Config) (*setup.SetupState, error) {
	state := &setup.SetupState{
		CurrentSettings: make(map[string]string),
	}

	if err := detectFlavor(ctx, conn, state); err != nil {
		return nil, err
	}

	if err := detectVersion(ctx, conn, state); err != nil {
		return nil, err
	}

	if err := detectSettings(ctx, conn, state); err != nil {
		return nil, err
	}

	if err := detectUserExists(ctx, conn, cfg.DatadogUser, state); err != nil {
		return nil, err
	}

	if err := detectDatabases(ctx, conn, cfg, state); err != nil {
		return nil, err
	}

	return state, nil
}

func detectFlavor(ctx context.Context, conn *pgx.Conn, state *setup.SetupState) error {
	// Check Cloud SQL first (distinct role).
	var cloudSQLHit int
	row := conn.QueryRow(ctx, sqlCheckCloudSQLRole)
	if err := row.Scan(&cloudSQLHit); err == nil && cloudSQLHit == 1 {
		state.Flavor = setup.FlavorCloudSQL
		return nil
	}

	// Check RDS/Aurora via rds.extensions setting.
	var rdsSetting *string
	row = conn.QueryRow(ctx, sqlCheckRDSSetting)
	if err := row.Scan(&rdsSetting); err == nil && rdsSetting != nil && *rdsSetting != "" {
		// Distinguish Aurora by version string.
		var versionStr string
		row2 := conn.QueryRow(ctx, sqlVersion)
		if err2 := row2.Scan(&versionStr); err2 == nil && strings.Contains(versionStr, "Aurora") {
			state.Flavor = setup.FlavorAurora
			return nil
		}
		state.Flavor = setup.FlavorRDS
		return nil
	}

	state.Flavor = setup.FlavorSelfHosted
	return nil
}

func detectVersion(ctx context.Context, conn *pgx.Conn, state *setup.SetupState) error {
	var versionNum string
	row := conn.QueryRow(ctx, sqlPGVersion)
	if err := row.Scan(&versionNum); err != nil {
		return err
	}
	// server_version_num is a 6-digit integer: e.g. "150006" for PG 15.6.
	// Major version = first two digits for < 10, first two for >= 10.
	n, err := strconv.Atoi(versionNum)
	if err != nil {
		return err
	}
	state.PGVersion = n / 10000
	return nil
}

func detectSettings(ctx context.Context, conn *pgx.Conn, state *setup.SetupState) error {
	rows, err := conn.Query(ctx, sqlGetSettings)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name, setting string
		var pendingRestart bool
		if err := rows.Scan(&name, &setting, &pendingRestart); err != nil {
			return err
		}
		state.CurrentSettings[name] = setting
		if pendingRestart {
			state.PendingRestart = append(state.PendingRestart, name)
		}
	}
	return rows.Err()
}

func detectUserExists(ctx context.Context, conn *pgx.Conn, username string, state *setup.SetupState) error {
	var hit int
	row := conn.QueryRow(ctx, sqlCheckUserExists, username)
	if err := row.Scan(&hit); err != nil {
		if strings.Contains(err.Error(), "no rows") {
			state.UserExists = false
			return nil
		}
		return err
	}
	state.UserExists = hit == 1
	return nil
}

func detectDatabases(ctx context.Context, conn *pgx.Conn, cfg setup.Config, state *setup.SetupState) error {
	if !cfg.AllDatabases {
		// cfg.Databases is already populated from the URI / --databases flag.
		state.Databases = cfg.Databases
		return nil
	}

	rows, err := conn.Query(ctx, sqlGetDatabases)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		state.Databases = append(state.Databases, name)
	}
	return rows.Err()
}
