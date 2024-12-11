// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"

	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/stretchr/testify/assert"
)

type vmSuiteEx2 struct {
	e2e.BaseSuite[environments.Host]
}

func TestVMSuiteEx2(t *testing.T) {
	e2e.Run(t, &vmSuiteEx2{}, e2e.WithProvisioner(
		awshost.ProvisionerNoAgentNoFakeIntake(
			awshost.WithEC2InstanceOptions(ec2.WithAMI("ami-05fab674de2157a80", os.AmazonLinux2, os.ARM64Arch), ec2.WithInstanceType("c6g.medium")),
		),
	))
}

func (v *vmSuiteEx2) TestAmiMatch() {
	ec2Metadata := client.NewEC2Metadata(v.T(), v.Env().RemoteHost.Host, v.Env().RemoteHost.OSFamily)
	amiID := ec2Metadata.Get("ami-id")
	assert.Equal(v.T(), amiID, "ami-05fab674de2157a80")
}
