// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package opener provides utilities to open log files with appropriate permissions.
package opener

import (
	"os"

	privilegedlogsclient "github.com/DataDog/datadog-agent/pkg/privileged-logs/client"
)

// OpenLogFile opens a file with the privileged logs client.
func OpenLogFile(path string) (*os.File, error) {
	return privilegedlogsclient.Open(path)
}

// StatLogFile stats a log file with the privileged logs client
func StatLogFile(path string) (os.FileInfo, error) {
	return privilegedlogsclient.Stat(path)
}
