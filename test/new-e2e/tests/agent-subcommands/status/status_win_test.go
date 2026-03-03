// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
)

type windowsStatusSuite struct {
	baseStatusSuite
}

func TestWindowsStatusSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &windowsStatusSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault))))))
}

func (v *windowsStatusSuite) TestStatusHostname() {
	metadata := client.NewEC2Metadata(v.T(), v.Env().RemoteHost.Host, v.Env().RemoteHost.OSFamily)
	resourceID := metadata.Get("instance-id")

	expectedSections := []expectedSection{
		{
			name:            `Hostname`,
			shouldBePresent: true,
			shouldContain:   []string{fmt.Sprintf("instance-id: %v", resourceID), "hostname provider: os"},
		},
	}

	fetchAndCheckStatus(&v.baseStatusSuite, expectedSections)
}

// This test asserts the presence of metadata sent by Python checks in the status subcommand output.
func (v *windowsStatusSuite) TestChecksMetadataWindows() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(
		awshost.WithRunOptions(
			ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault)),
			ec2.WithAgentOptions(
				agentparams.WithIntegration("custom_check.d", string(customCheckYaml)),
				agentparams.WithFile("C:/ProgramData/Datadog/checks.d/custom_check.py", string(customCheckPython), true),
			),
		),
	))

	expectedSections := []expectedSection{
		{
			name:            "Collector",
			shouldBePresent: true,
			shouldContain: []string{"Instance ID:", "[OK]",
				// Following lines check the presence of checks metadata
				"metadata:",
				"custom_metadata_key: custom_metadata_value",
			},
		},
	}

	fetchAndCheckStatus(&v.baseStatusSuite, expectedSections)
}

func (v *windowsStatusSuite) TestDefaultInstallStatus() {
	v.testDefaultInstallStatus(nil, []string{"Status: Not running or unreachable"})
}
