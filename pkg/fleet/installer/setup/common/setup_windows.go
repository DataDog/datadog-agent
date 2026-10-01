// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package common

import "github.com/DataDog/datadog-agent/pkg/fleet/installer/msi"

// CheckAgentFlavor validates compatibility when setup will install the Agent.
func (s *Setup) CheckAgentFlavor() error {
	for _, pkg := range resolvePackages(s.Env, s.Packages) {
		if pkg.name == DatadogAgentPackage {
			return msi.CheckAgentFlavor(s.Env.FIPSMode)
		}
	}
	return nil
}

func (s *Setup) postInstallPackages() (err error) {
	// nothing to do on windows
	return nil
}

func copyInstallerSSI() error {
	// nothing to do on windows
	return nil
}
