// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

type windowsStatusSuite struct {
	baseStatusSuite
	osOption awshost.ProvisionerOption
}

func (v *windowsStatusSuite) GetOs() awshost.ProvisionerOption {
	return v.osOption
}

func TestWindowsStatusSuite(t *testing.T) {
	osOption := awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault))
	e2e.Run(t, &windowsStatusSuite{osOption: osOption}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(osOption)))
}

func (v *windowsStatusSuite) TestStatusHostname() {

	config := agentparams.WithAgentConfig("ec2_prefer_imdsv2: true")
	options := awshost.WithAgentOptions(config)
	provisioner := awshost.ProvisionerNoFakeIntake(v.GetOs(), options)
	v.UpdateEnv(provisioner)
	// e2e metadata provider already uses IMDSv2
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
