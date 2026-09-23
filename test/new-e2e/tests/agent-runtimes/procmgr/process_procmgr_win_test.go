// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	processProcessName           = "datadog-agent-process"
	processLegacySCMServiceName  = "datadog-process-agent"
	processProcmgrConfigFileName = "datadog-agent-process.yaml"
)

type processProcmgrWindowsSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestProcessAgentManagedByProcmgrWindows(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &processProcmgrWindowsSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.WindowsServerDefault)),
				ec2.WithAgentOptions(),
			),
		),
	))
}

// TestProcessAgentSupervisedByProcmgrAndLegacySCMStopped is the end-to-end proof of the
// Windows cutover: on a default install dd-procmgr brings process-agent up on its own, and
// the core Agent leaves the legacy SCM service alone. Both halves have to hold at once.
// Either one alone is a bug: only the first means two process-agents, only the second means
// none at all.
func (s *processProcmgrWindowsSuite) TestProcessAgentSupervisedByProcmgrAndLegacySCMStopped() {
	host := s.Env().RemoteHost
	installRoot, err := windowsagent.GetInstallPathFromRegistry(host)
	require.NoError(s.T(), err)

	// Unlike PAR, process-agent ships with every install, so a missing binary is a failure
	// rather than a reason to skip.
	processBin := filepath.Join(installRoot, "bin", "agent", "process-agent.exe")
	exists, err := host.FileExists(processBin)
	require.NoError(s.T(), err)
	require.True(s.T(), exists, "process-agent.exe should be installed at %s", processBin)

	cfg := filepath.Join(installRoot, "processes.d", processProcmgrConfigFileName)
	exists, err = host.FileExists(cfg)
	require.NoError(s.T(), err)
	require.True(s.T(), exists, "fleet process-agent processes.d config should exist at %s", cfg)

	cli := filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe")
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(fmt.Sprintf(`& "%s" describe %s`, cli, processProcessName))
		assert.NoError(ct, err)
		assert.Contains(ct, out, "State")
		assert.Contains(ct, out, "Running")
	}, 120*time.Second, 3*time.Second)

	out, err := host.Execute(fmt.Sprintf(
		`$s = Get-Service -Name '%s' -ErrorAction SilentlyContinue; if ($null -eq $s) { 'Absent' } else { $s.Status }`,
		processLegacySCMServiceName,
	))
	require.NoError(s.T(), err)
	require.NotEqual(s.T(), "Running", strings.TrimSpace(out),
		"%s Windows service must not be Running when process-agent is managed by dd-procmgr", processLegacySCMServiceName)
}
