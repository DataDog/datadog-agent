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
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/stretchr/testify/assert"
)

type myVMSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestMyVMSuite(t *testing.T) {
	e2e.Run(t, &myVMSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOSArch(os.AmazonLinux2023, os.ARM64Arch)))))
}

func (v *myVMSuite) TestIsAmazonLinux() {
	res := v.Env().RemoteHost.MustExecute("cat /etc/os-release")
	assert.Contains(v.T(), res, "Amazon Linux")
}
