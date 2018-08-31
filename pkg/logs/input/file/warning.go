// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package file

import "fmt"

// OpenFilesLimitWarningType is the key that identify OpenFilesLimitWarning in warning
const OpenFilesLimitWarningType = "open_files_limit_warning"

// OpenFilesLimitWarning handles the case when too many files are tailed
type OpenFilesLimitWarning struct {
	filesLimit int
}

// NewOpenFilesLimitWarning initialize an OpenFilesLimitWarning
func NewOpenFilesLimitWarning(filesLimit int) *OpenFilesLimitWarning {
	return &OpenFilesLimitWarning{
		filesLimit: filesLimit,
	}
}

// Render prints the warning message
func (w *OpenFilesLimitWarning) Render() string {
	return fmt.Sprintf(
		"The limit on the maximum number of files in use (%d) has been reached. Increase this limit (thanks to the attribute logs_config.open_files_limit in datadog.yaml) or decrease the number of tailed file.",
		w.filesLimit,
	)
}
