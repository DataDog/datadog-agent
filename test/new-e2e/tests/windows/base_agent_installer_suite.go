// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package windows contains the code to run the e2e tests on Windows
package windows

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	platformCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"path/filepath"
)

// BaseAgentInstallerSuite is a base class for the Windows Agent installer suites
type BaseAgentInstallerSuite[Env any] struct {
	e2e.BaseSuite[Env]

	AgentPackage *windowsAgent.Package
	OutputDir    string
}

// InstallAgent installs the Agent on a given Windows host. It will pass all the parameters to the MSI installer.
func (b *BaseAgentInstallerSuite[Env]) InstallAgent(host *components.RemoteHost, options ...windowsAgent.InstallAgentOption) (string, error) {
	b.T().Helper()
	opts := []windowsAgent.InstallAgentOption{
		windowsAgent.WithInstallLogFile(filepath.Join(b.OutputDir, "install.log")),
	}
	opts = append(opts, options...)
	return windowsAgent.InstallAgent(host, opts...)
}

// NewTestClientForHost creates a new TestClient for a given host.
func (b *BaseAgentInstallerSuite[Env]) NewTestClientForHost(host *components.RemoteHost) *platformCommon.TestClient {
	// We could bring the code from NewWindowsTestClient here
	return platformCommon.NewWindowsTestClient(b.T(), host)
}

// BeforeTest overrides the base BeforeTest to perform some additional per-test setup like configuring the output directory.
func (b *BaseAgentInstallerSuite[Env]) BeforeTest(suiteName, testName string) {
	b.BaseSuite.BeforeTest(suiteName, testName)

	var err error
	b.OutputDir, err = runner.GetTestOutputDir(runner.GetProfile(), b.T())
	if err != nil {
		b.T().Fatalf("should get output dir")
	}
	b.T().Logf("Output dir: %s", b.OutputDir)
}

// SetupSuite overrides the base SetupSuite to perform some additional setups like setting the package to install.
func (b *BaseAgentInstallerSuite[Env]) SetupSuite() {
	b.BaseSuite.SetupSuite()

	var err error
	b.AgentPackage, err = b.GetAgentPackage()
	if err != nil {
		b.T().Fatal(err)
	}
	b.T().Logf("Using Agent: %#v", b.AgentPackage)
}

// GetAgentPackage returns the Agent package to install.
// This method is called automatically by SetupSuite, and only needs to be called explicitly
// if you need to get the package before SetupSuite is called.
func (b *BaseAgentInstallerSuite[Env]) GetAgentPackage() (*windowsAgent.Package, error) {
	if b.AgentPackage == nil {
		var err error
		b.AgentPackage, err = windowsAgent.GetPackageFromEnv()
		if err != nil {
			return nil, fmt.Errorf("failed to get MSI URL from env: %w", err)
		}
	}

	return b.AgentPackage, nil
}
