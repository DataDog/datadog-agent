// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/stretchr/testify/require"
)

type vmUpdaterSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestUpdaterSuite(t *testing.T) {
	e2e.Run(t, &vmUpdaterSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
		awshost.WithUpdater(),
		awshost.WithEC2InstanceOptions(ec2.WithOSArch(os.UbuntuDefault, os.ARM64Arch)),
	)))
}

func (v *vmUpdaterSuite) TestUpdaterOwnership() {
	// updater user exists and is a system user
	require.Equal(v.T(), "/usr/sbin/nologin\n", v.Env().RemoteHost.MustExecute(`getent passwd dd-agent | cut -d: -f7`), "unexpected: user does not exist or is not a system user")
	// owner
	require.Equal(v.T(), "dd-agent\n", v.Env().RemoteHost.MustExecute(`stat -c "%U" /opt/datadog/updater`))
	// group
	require.Equal(v.T(), "dd-agent\n", v.Env().RemoteHost.MustExecute(`stat -c "%G" /opt/datadog/updater`))
	// permissions
	require.Equal(v.T(), "drwxr-xr-x\n", v.Env().RemoteHost.MustExecute(`stat -c "%A" /opt/datadog/updater`))
}

func (v *vmUpdaterSuite) TestUpdaterAdmin() {
	adminPathUnit := "datadog-updater-admin.path"
	require.Equal(v.T(), "enabled\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`systemctl is-enabled %s`, adminPathUnit)))
	require.Equal(v.T(), "active\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`systemctl is-active %s`, adminPathUnit)))

	/*
		temporary while fixing locally, review can start without
			// admin should activate on fifo creation
			v.Env().RemoteHost.MustExecute("sudo mkfifo /opt/datadog/updater/run/out.fifo && sudo mkfifo /opt/datadog/updater/run/in.fifo")
			v.Env().RemoteHost.MustExecute("sudo chmod 0666 /opt/datadog/updater/run/out.fifo && sudo chmod 0666 /opt/datadog/updater/run/in.fifo")

			// assert failures
			expectedResults := map[string]string{
				"&":                                      "invalid command: &",
				";":                                      "invalid command: ;",
				"start datadog agent /":                  "invalid command: start datadog agent /",
				"start datadog agent *":                  "invalid command: start datadog agent *",
				string(make([]byte, 101)):                "command longer than 100 characters",
				"not-supported":                          "not supported command: not-supported",
				"start":                                  "missing argument",
				"enable":                                 "missing argument",
				"disable":                                "missing argument",
				"stop":                                   "missing argument",
				"load-unit":                              "missing argument",
				"remove-unit":                            "missing argument",
				"start non-datadog-unit.service *":       "unit name must start with 'datadog'",
				"enable non-datadog-unit.service *":      "unit name must start with 'datadog'",
				"disable non-datadog-unit.service *":     "unit name must start with 'datadog'",
				"stop non-datadog-unit.service *":        "unit name must start with 'datadog'",
				"load-unit non-datadog-unit.service *":   "unit name must start with 'datadog'",
				"remove-unit non-datadog-unit.service *": "unit name must start with 'datadog'",
			}
			for cmd, expected := range expectedResults {
				v.Env().RemoteHost.MustExecute(fmt.Sprintf(`echo %s > /opt/datadog/updater/run/in.fifo`, cmd))
				require.Equal(v.T(), v.Env().RemoteHost.MustExecute(`< /opt/datadog/updater/run/out.fifo`), expected)
			}

			v.Env().RemoteHost.MustExecute("rm /opt/datadog/updater/run/in.fifo")
			time.Sleep(5 * time.Second)
	*/
}

func (v *vmUpdaterSuite) TestAgentUnitsLoaded() {
	stableUnits := []string{
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-agent-sysprobe.service",
		"datadog-agent-security.service",
	}
	for _, unit := range stableUnits {
		require.Equal(v.T(), "enabled\n", v.Env().RemoteHost.MustExecute(fmt.Sprintf(`systemctl is-enabled %s`, unit)))
	}
}
