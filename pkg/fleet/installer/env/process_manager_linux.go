// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

import (
	"os"
	"strings"
)

// ProcessManagerEnvFilePath is the shared environment override file that the installer daemon's
// own systemd units source via EnvironmentFile= (see packages.SetProcessManagerEnabled /
// persistProcessManagerEnabled, which write to it). Declared here, in the lowest-level package,
// so that ProcessManagerEnabledFromEnv can fall back to the last persisted choice for every
// caller — not just the installer daemon's own process — without pkg/fleet/installer/env
// depending on the higher-level pkg/fleet/installer/packages package. Overridable in tests.
var ProcessManagerEnvFilePath = "/opt/datadog-packages/run/environment"

func readPersistedProcessManagerEnabled() (bool, bool) {
	existing, err := os.ReadFile(ProcessManagerEnvFilePath)
	if err != nil {
		return false, false
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if v, ok := strings.CutPrefix(line, envProcessManagerEnabled+"="); ok {
			return strings.EqualFold(strings.TrimSpace(v), "true"), true
		}
	}
	return false, false
}
