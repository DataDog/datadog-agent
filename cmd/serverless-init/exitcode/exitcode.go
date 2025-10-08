// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package exitcode

import (
	"errors"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// From returns the process exit code embedded in err.
// If err is nil, 0 is returned. If no exit code is found, it falls back to 1.
func From(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}

	log.Debug("using default exit code 1")
	return 1
}
