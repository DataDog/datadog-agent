// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"path"
)

// BaseInstallerSuite the base suite for all Datadog Installer tests.
type BaseInstallerSuite struct {
	BaseSuite
	installer *DatadogInstaller
}

// Installer the Datadog Installer for testing.
func (s *BaseInstallerSuite) Installer() *DatadogInstaller {
	return s.installer
}

// BeforeTest creates a new Datadog Installer and sets the output logs directory for each tests
func (s *BaseInstallerSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	s.ensureDirs()
	s.installer = NewDatadogInstaller(s.Env(), s.SessionOutputDir())
}

// ensureDirs enforces the required dirs are created.
// They should be created by the powershell script but here we use the MSI; so this
// is a quick fix with hardcoded paths that should be removed once the powershell
// script is used.
func (s *BaseInstallerSuite) ensureDirs() {
	basePath := "C:/ProgramData/Datadog Installer"
	paths := []string{
		path.Join(basePath, "packages"),
		path.Join(basePath, "configs"),
		path.Join(basePath, "locks"),
		path.Join(basePath, "tmp"),
	}
	for _, p := range paths {
		s.Env().RemoteHost.MustExecute(fmt.Sprintf("New-Item -Path \"%s\" -ItemType Directory -Force", p))
	}
}
