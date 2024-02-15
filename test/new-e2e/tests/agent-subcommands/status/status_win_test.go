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
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

type windowsStatusSuite struct {
	baseStatusSuite
}

func TestWindowsStatusSuite(t *testing.T) {
	e2e.Run(t, &windowsStatusSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)), awshost.WithAgentOptions(agentparams.WithPipeline("28299613")))))
}

func (v *windowsStatusSuite) TestStatusHostname() {
	metadata := client.NewEC2Metadata(v.Env().RemoteHost)
	resourceID := metadata.Get("instance-id")

	status := v.Env().Agent.Client.Status()

	expected := expectedSection{
		name:            `Hostname`,
		shouldBePresent: true,
		shouldContain:   []string{fmt.Sprintf("instance-id: %v", resourceID), "hostname provider: os"},
	}

	verifySectionContent(v.T(), status.Content, expected)
}
