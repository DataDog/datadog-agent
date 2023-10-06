// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package runner

import (
	"github.com/DataDog/datadog-agent/pkg/collector/python"
)

func terminateChecksRunningProcesses() {
	// stop all python subprocesses
	python.TerminateRunningProcesses()
}
