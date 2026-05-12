// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/internal/procmgrtest"
)

const (
	linuxDaemonBin = "/opt/datadog-agent/embedded/bin/dd-procmgrd"
	linuxConfigDir = "/opt/datadog-agent/processes.d"

	linuxTestProcessConfig = `command: /bin/sleep
args:
  - "3600"
auto_start: true
restart: always
description: E2E test process
`

	linuxMissingBinaryConfig = `command: /nonexistent/binary
condition_path_exists: /nonexistent/binary
auto_start: true
restart: never
description: should not start
`
)

var linuxPlatform = platformConfig{
	daemonBin:         linuxDaemonBin,
	cliBin:            procmgrtest.CLIBinDefault,
	configDir:         linuxConfigDir,
	sleepCommand:      "/bin/sleep",
	testProcessYAML:   linuxTestProcessConfig,
	missingBinaryYAML: linuxMissingBinaryConfig,
	checkBinCmd:       func(path string) string { return "test -f " + path },
	checkSvcRunning:   "systemctl is-active datadog-agent-procmgr",
	svcRunningOutput:  "active",
	// Run CLI as dd-agent so it can use the procmgrd socket without chmod (same as installer DDOT tests).
	cliCmd: func(args string) string {
		return fmt.Sprintf("sudo -u dd-agent -- %q %s", procmgrtest.CLIBinDefault, args)
	},
}

// procmgrSmokeLinuxSuite runs generic dd-procmgr checks (sleep process, list/describe, etc.)
// without installing the DDOT extension. See procmgr_extension_ddot_nix_test.go for DDOT.
type procmgrSmokeLinuxSuite struct {
	baseProcmgrSuite
}

func (s *procmgrSmokeLinuxSuite) SetupSuite() {
	s.baseProcmgrSuite.SetupSuite()
}

func TestProcmgrSmokeLinuxSuite(t *testing.T) {
	t.Parallel()
	s := &procmgrSmokeLinuxSuite{}
	s.platform = linuxPlatform
	e2e.Run(t, s, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithFile(linuxConfigDir+"/test-sleep.yaml", linuxTestProcessConfig, true),
					agentparams.WithFile(linuxConfigDir+"/missing-binary.yaml", linuxMissingBinaryConfig, true),
				),
			),
		),
	))
}
