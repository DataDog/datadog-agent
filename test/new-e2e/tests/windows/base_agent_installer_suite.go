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
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	infraCommon "github.com/DataDog/test-infra-definitions/common"
	"path/filepath"
	"reflect"
	"strings"
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
	p, err := infraCommon.ApplyOption(&windowsAgent.InstallAgentParams{
		InstallLogFile: "install.log",
	}, options)
	if err != nil {
		return "", err
	}

	if p.Package == nil {
		return "", fmt.Errorf("missing agent package to install")
	}

	var args []string
	typeOfInstallAgentParams := reflect.TypeOf(p)
	for fieldIndex := 0; fieldIndex < typeOfInstallAgentParams.NumField(); fieldIndex++ {
		field := typeOfInstallAgentParams.Field(fieldIndex)
		installerArg := field.Tag.Get("installer_arg")
		if installerArg != "" {
			installerArgValue := reflect.ValueOf(p).FieldByName(field.Name).String()
			if installerArgValue != "" {
				args = append(args, fmt.Sprintf("%s=%s", installerArg, installerArgValue))
			}
		}
	}

	remoteMSIPath, err := common.GetTemporaryFile(host)
	if err != nil {
		return "", err
	}
	err = common.PutOrDownloadFile(host, p.Package.URL, remoteMSIPath)
	if err != nil {
		return "", err
	}

	return remoteMSIPath, common.InstallMSI(host, remoteMSIPath, strings.Join(args, " "), filepath.Join(b.OutputDir, p.InstallLogFile))
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
	b.AgentPackage, err = windowsAgent.GetPackageFromEnv()
	if err != nil {
		b.T().Fatalf("failed to get MSI URL from env: %v", err)
	}
	b.T().Logf("Using Agent: %#v", b.AgentPackage)
}
