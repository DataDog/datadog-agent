// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && functionaltests

// Package trivy holds the scan components
package trivy

import (
	// used to read RPM database
	// mattn/go-sqlite3 is currently not fully supported by our functional tests setup
	_ "modernc.org/sqlite"
)
