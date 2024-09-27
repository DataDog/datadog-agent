// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"fmt"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

type windowsStatusSuite struct {
	baseStatusSuite
}

func TestWindowsStatusSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &windowsStatusSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)))))
}

func (v *windowsStatusSuite) TestStatusHostname() {
	metadata := client.NewEC2Metadata(v.T(), v.Env().RemoteHost.Host, v.Env().RemoteHost.OSFamily)
	resourceID := metadata.Get("instance-id")

	status := v.Env().Agent.Client.Status()

	expected := expectedSection{
		name:            `Hostname`,
		shouldBePresent: true,
		shouldContain:   []string{fmt.Sprintf("instance-id: %v", resourceID), "hostname provider: os"},
	}

	verifySectionContent(v.T(), status.Content, expected)
}

// This test asserts the presence of metadata sent by Python checks in the status subcommand output.
func (v *windowsStatusSuite) TestChecksMetadataWindows() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		awshost.WithAgentOptions(
			agentparams.WithFile("C:/ProgramData/Datadog/conf.d/custom_check.d/conf.yaml", string(customCheckYaml), true),
			agentparams.WithFile("C:/ProgramData/Datadog/checks.d/custom_check.py", string(customCheckPython), true),
		)))

	section := expectedSection{
		name:            "Collector",
		shouldBePresent: true,
		shouldContain: []string{"Instance ID:", "[OK]",
			// Following lines check the presence of checks metadata
			"metadata:",
			"custom_metadata_key: custom_metadata_value",
		},
	}

	status := v.Env().Agent.Client.Status()
	verifySectionContent(v.T(), status.Content, section)
}
