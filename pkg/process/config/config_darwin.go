// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"path/filepath"
)

const (
	defaultLogFilePath = "/opt/datadog-agent/logs/process-agent.log"

	// Agent 6
	defaultDDAgentBin = "/opt/datadog-agent/bin/agent/agent"

	// defaultSystemProbeAddress is the default unix socket path to be used for connecting to the system probe
	defaultSystemProbeAddress = "/opt/datadog-agent/run/sysprobe.sock"
)

// ValidateSysprobeSocket validates that the sysprobe socket config option is of the correct format.
func ValidateSysprobeSocket(sockPath string) error {
	if !filepath.IsAbs(sockPath) {
		return fmt.Errorf("socket path must be an absolute file path")
	}
	return nil
}
