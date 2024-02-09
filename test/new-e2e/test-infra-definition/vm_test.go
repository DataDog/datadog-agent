// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"

	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/require"
)

const (
	requestedAmi = "ami-05fab674de2157a80"
	instanceType = "t3.medium"
	userDataPath = "/tmp/test-userdata"
)

type vmBaseSuite struct {
	e2e.BaseSuite[environments.Host]
}

type vmSuiteWithAMI struct {
	vmBaseSuite
}

type vmSuiteWithInstanceType struct {
	vmBaseSuite
}

type vmSuiteWithArch struct {
	vmBaseSuite
}

type vmSuiteWithUserData struct {
	vmBaseSuite
}

type vmTestCase struct {
	testName    string
	provisioner e2e.Provisioner
	suite       e2e.Suite[environments.Host]
}

func TestVMSuite(t *testing.T) {
	testCases := []vmTestCase{
		{
			testName:    "testWithAMI",
			provisioner: awshost.ProvisionerNoAgentNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithAMI(requestedAmi, os.AmazonLinux2, os.ARM64Arch))),
			suite:       &vmSuiteWithAMI{},
		},

		{
			testName:    "testWithInstanceType",
			provisioner: awshost.ProvisionerNoAgentNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithInstanceType(instanceType))),
			suite:       &vmSuiteWithInstanceType{},
		},
		{
			testName:    "testWithArch",
			provisioner: awshost.ProvisionerNoAgentNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithOSArch(os.DebianDefault, os.ARM64Arch))),
			suite:       &vmSuiteWithArch{},
		},
		{
			testName:    "testWithUserData",
			provisioner: awshost.ProvisionerNoAgentNoFakeIntake(awshost.WithEC2InstanceOptions(ec2.WithUserData("#!/bin/bash\ntouch " + userDataPath))),
			suite:       &vmSuiteWithUserData{},
		},
	}
	for _, tc := range testCases {
		t.Log(tc.testName)
		e2e.Run(t, tc.suite, e2e.WithProvisioner(tc.provisioner))
	}
}

func (v *vmSuiteWithAMI) TestWithImageName() {
	vm := v.Env().RemoteHost
	metadata := client.NewEC2Metadata(vm)
	require.Equal(v.T(), requestedAmi, metadata.Get("ami-id"))
	require.Equal(v.T(), "aarch64\n", vm.MustExecute("uname -m"))
	require.Contains(v.T(), vm.MustExecute("grep PRETTY_NAME /etc/os-release"), "Amazon Linux")
}

func (v *vmSuiteWithInstanceType) TestWithInstanceType() {
	vm := v.Env().RemoteHost
	metadata := client.NewEC2Metadata(vm)
	require.Equal(v.T(), metadata.Get("instance-type"), instanceType)
}

func (v *vmSuiteWithArch) TestWithArch() {
	require.Equal(v.T(), "aarch64\n", v.Env().RemoteHost.MustExecute("uname -m"))
}

func (v *vmSuiteWithUserData) TestWithUserdata() {
	v.UpdateEnv(awshost.Provisioner(awshost.WithoutAgent(), awshost.WithEC2InstanceOptions(ec2.WithUserData("#!/bin/bash\ntouch "+userDataPath))))

	output, err := v.Env().RemoteHost.Execute("ls " + userDataPath)
	require.NoError(v.T(), err)
	require.Equal(v.T(), userDataPath+"\n", output)
}
