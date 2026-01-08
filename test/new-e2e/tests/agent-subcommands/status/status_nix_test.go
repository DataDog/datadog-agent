// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
)

type linuxStatusSuite struct {
	baseStatusSuite
}

func TestLinuxStatusSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &linuxStatusSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(awshost.WithRunOptions(scenec2.WithoutFakeIntake()))))
}

func (v *linuxStatusSuite) TestStatusHostname() {
	metadata := client.NewEC2Metadata(v.T(), v.Env().RemoteHost.Host, v.Env().RemoteHost.OSFamily)
	resourceID := metadata.Get("instance-id")

	expectedSections := []expectedSection{
		{
			name:            `Hostname`,
			shouldBePresent: true,
			shouldContain:   []string{fmt.Sprintf("hostname: %v", resourceID), "hostname provider: aws"},
		},
	}

	fetchAndCheckStatus(&v.baseStatusSuite, expectedSections)
}

func (v *linuxStatusSuite) TestFIPSProxyStatus() {
	var shouldContains []string
	var shouldNotContains []string

	if v.Env().Agent.FIPSEnabled {
		// FIPS-enabled Agent should never display FIPS Proxy status
		shouldContains = []string{"FIPS Mode: enabled"}
		shouldNotContains = []string{"FIPS Mode: proxy", "FIPS proxy"}
	} else {
		// Non-FIPS Agent should print FIPS Proxy status
		shouldContains = []string{"FIPS Mode: proxy", "FIPS proxy"}
	}

	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig("fips.enabled: true")))))

	expectedSections := []expectedSection{
		{
			name:             `Agent \(.*\)`,
			shouldBePresent:  true,
			shouldContain:    shouldContains,
			shouldNotContain: shouldNotContains,
		},
	}

	fetchAndCheckStatus(&v.baseStatusSuite, expectedSections)
}

// This test asserts the presence of metadata sent by Python checks in the status subcommand output.
func (v *linuxStatusSuite) TestChecksMetadataUnix() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithRunOptions(scenec2.WithAgentOptions(
		agentparams.WithIntegration("custom_check.d", string(customCheckYaml)),
		agentparams.WithFile("/etc/datadog-agent/checks.d/custom_check.py", string(customCheckPython), true),
	))))

	expectedSections := []expectedSection{
		{
			name:            "Collector",
			shouldBePresent: true,
			shouldContain: []string{"Instance ID:", "[OK]",
				"metadata:",
				"custom_metadata_key: custom_metadata_value",
			},
		},
	}

	fetchAndCheckStatus(&v.baseStatusSuite, expectedSections)
}

func (v *linuxStatusSuite) TestDefaultInstallStatus() {
	// wake up the trace-agent
	v.Env().RemoteHost.NewHTTPClient().Get("http://localhost:8126/services")
	v.testDefaultInstallStatus([]string{"Status: Not running or unreachable"}, nil)
}
