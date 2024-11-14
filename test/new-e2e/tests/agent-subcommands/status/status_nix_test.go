// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
)

type linuxStatusSuite struct {
	baseStatusSuite
}

func TestLinuxStatusSuite(t *testing.T) {
	e2e.Run(t, &linuxStatusSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

func (v *linuxStatusSuite) TestStatusHostname() {
	metadata := client.NewEC2Metadata(v.T(), v.Env().RemoteHost.Host, v.Env().RemoteHost.OSFamily)
	resourceID := metadata.Get("instance-id")

	status := v.Env().Agent.Client.Status()

	expected := expectedSection{
		name:            `Hostname`,
		shouldBePresent: true,
		shouldContain:   []string{fmt.Sprintf("hostname: %v", resourceID), "hostname provider: aws"},
	}

	verifySectionContent(v.T(), status.Content, expected)
}

func (v *linuxStatusSuite) TestFIPSProxyStatus() {
	v.UpdateEnv(awshost.ProvisionerNoFakeIntake(awshost.WithAgentOptions(agentparams.WithAgentConfig("fips.enabled: true"))))

	expectedSection := expectedSection{
		name:            `Agent \(.*\)`,
		shouldBePresent: true,
		shouldContain:   []string{"FIPS proxy"},
	}
	status := v.Env().Agent.Client.Status()
	verifySectionContent(v.T(), status.Content, expectedSection)
}
