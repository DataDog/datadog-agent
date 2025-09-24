// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultscript

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
)

// SetupAPMSSIScript sets up the APM SSI installation script.
func SetupAPMSSIScript(s *common.Setup) error {
	// Telemetry
	telemetrySupportedEnvVars(s, supportedEnvVars...)

	// Installer management
	setConfigInstallerRegistries(s)

	// Install packages
	installAPMPackages(s)

	// Copy installer to /usr/bin/datadog-installer if agent isn't installed
	if _, noInstallAgent := os.LookupEnv("DD_NO_AGENT_INSTALL"); noInstallAgent {
		s.Packages.WriteSSIInstaller()
	}

	return nil
}
