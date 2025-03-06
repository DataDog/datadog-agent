// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && !functionaltests

// Package trivy holds the scan components
package trivy

import (
	// used to read RPM database
	"database/sql"

	"github.com/mattn/go-sqlite3"
)

// This is required to load sqlite based RPM databases
func init() {
	// mattn/go-sqlite3 is only registering the sqlite3 driver
	// let's register the sqlite (no 3) driver as well
	sql.Register("sqlite", &sqlite3.SQLiteDriver{})
}
