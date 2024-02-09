// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package windows

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	infraCommon "github.com/DataDog/test-infra-definitions/common"
	"path/filepath"
	"strings"
)

type BaseAgentInstallerSuite[Env any] struct {
	e2e.BaseSuite[Env]

	AgentPackage *windowsAgent.Package
	OutputDir    string
}

func (b *BaseAgentInstallerSuite[Env]) InstallAgent(host *components.RemoteHost, options ...windowsAgent.InstallAgentOption) (string, error) {
	p := &windowsAgent.InstallAgentParams{
		InstallLogFile: "install.log",
	}
	infraCommon.ApplyOption(p, options)

	if p.Package == nil {
		return "", fmt.Errorf("missing agent package to install")
	}

	var args []string

	if p.AgentUser != "" {
		args = append(args, fmt.Sprintf("DDAGENTUSER_NAME=%s", p.AgentUser))
	}
	if p.AgentUserPassword != "" {
		args = append(args, fmt.Sprintf("DDAGENTUSER_PASSWORD=%s", p.AgentUserPassword))
	}
	if p.Site != "" {
		args = append(args, fmt.Sprintf("SITE=%s", p.Site))
	}
	if p.ApiKey != "" {
		args = append(args, fmt.Sprintf("APIKEY=%s", p.ApiKey))
	}
	if p.DdUrl != "" {
		args = append(args, fmt.Sprintf("DD_URL=%s", p.DdUrl))
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

func (b *BaseAgentInstallerSuite[Env]) NewAgentClientForHost(host *components.RemoteHost) agentclient.Agent {
	agentClient, err := client.NewHostAgentClient(b.T(), host, true)
	if err != nil {
		b.T().Fatalf("should get host agent client")
	}
	return agentClient
}

func (b *BaseAgentInstallerSuite[Env]) SetupSuite() {
	b.BaseSuite.SetupSuite()

	var err error
	b.OutputDir, err = runner.GetTestOutputDir(runner.GetProfile(), b.T())
	if err != nil {
		b.T().Fatalf("should get output dir")
	}
	b.T().Logf("Output dir: %s", b.OutputDir)

	b.AgentPackage, err = windowsAgent.GetPackageFromEnv()
	if err != nil {
		b.T().Fatalf("failed to get MSI URL from env: %v", err)
	}
	b.T().Logf("Using Agent: %#v", b.AgentPackage)
}
