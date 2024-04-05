// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build podman

package podman

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"

	// SQLite backend for database/sql
	_ "modernc.org/sqlite"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Same strategy as for BoltDB : we do not need the full podman go package.
// This reduces the number of dependencies and the size of the ultimately shipped binary.
//
// The functions in this file have been copied from
// https://github.com/containers/podman/blob/v5.0.0/libpod/sqlite_state.go
// The code has been adapted a bit to our needs. The only functions of that file
// that we need are AllContainers() and NewSqliteState().
//
// This code could break in future versions of Podman. This has been tried with
// v4.9.2 and v5.0.0.

// SQLDBClient is a client for the podman's state database in the SQLite format.
type SQLDBClient struct {
	DBPath string
}

const (
	// Deal with timezone automatically.
	sqliteOptionLocation = "_loc=auto"
	// Read-only mode (https://www.sqlite.org/pragma.html#pragma_query_only)
	sqliteOptionQueryOnly = "&_query_only=true"
	// Make sure busy timeout is set to high value to keep retrying when the db is locked.
	// Timeout is in ms, so set it to 100s to have enough time to retry the operations.
	sqliteOptionBusyTimeout = "&_busy_timeout=100000"

	// Assembled sqlite options used when opening the database.
	sqliteOptions = "?" + sqliteOptionLocation + sqliteOptionQueryOnly + sqliteOptionBusyTimeout
)

// NewSQLDBClient returns a DB client that uses the DB stored in dbPath.
func NewSQLDBClient(dbPath string) *SQLDBClient {
	return &SQLDBClient{
		DBPath: dbPath,
	}
}

// getDBCon opens a connection to the SQLite-backed state database.
// Note: original function comes from https://github.com/containers/podman/blob/e71ec6f1d94d2d97fb3afe08aae0d8adaf8bddf0/libpod/sqlite_state.go#L57-L96
// It was adapted as we don't need to write any information to the DB.
func (client *SQLDBClient) getDBCon() (*sql.DB, error) {
	conn, err := sql.Open("sqlite", filepath.Join(client.DBPath, sqliteOptions))
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}
	return conn, nil
}

// GetAllContainers retrieves all the containers in the database.
// We retrieve the state always.
func (client *SQLDBClient) GetAllContainers() ([]Container, error) {
	var res []Container

	conn, err := client.getDBCon()
	if err != nil {
		return nil, err
	}
	defer func() {
		if errClose := conn.Close(); errClose != nil {
			log.Warnf("failed to close sqlite db: %q", err)
		}
	}()

	rows, err := conn.Query("SELECT ContainerConfig.JSON, ContainerState.JSON AS StateJSON FROM ContainerConfig INNER JOIN ContainerState ON ContainerConfig.ID = ContainerState.ID;")
	if err != nil {
		return nil, fmt.Errorf("retrieving all containers from database: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var configJSON, stateJSON string
		if err := rows.Scan(&configJSON, &stateJSON); err != nil {
			return nil, fmt.Errorf("scanning container from database: %w", err)
		}

		ctr := new(Container)
		ctr.Config = new(ContainerConfig)
		ctr.State = new(ContainerState)

		if err := json.Unmarshal([]byte(configJSON), ctr.Config); err != nil {
			return nil, fmt.Errorf("unmarshalling container config: %w", err)
		}
		if err := json.Unmarshal([]byte(stateJSON), ctr.State); err != nil {
			return nil, fmt.Errorf("unmarshalling container %s state: %w", ctr.Config.ID, err)
		}

		res = append(res, *ctr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return res, nil
}
