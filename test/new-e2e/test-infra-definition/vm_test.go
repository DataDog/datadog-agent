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

type vmSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestVMSuite(t *testing.T) {
	e2e.Run(t, &vmSuite{})
}

func (v *vmSuite) TestWithImageName() {
	requestedAmi := "ami-05fab674de2157a80"
	v.UpdateEnv(awshost.Provisioner(awshost.WithoutAgent(), awshost.WithEC2InstanceOptions(ec2.WithAMI(requestedAmi, os.AmazonLinux2, os.ARM64Arch))))

	vm := v.Env().RemoteHost
	metadata := client.NewEC2Metadata(vm)
	require.Equal(v.T(), requestedAmi, metadata.Get("ami-id"))
	require.Equal(v.T(), "aarch64\n", vm.MustExecute("uname -m"))
	require.Contains(v.T(), vm.MustExecute("grep PRETTY_NAME /etc/os-release"), "Amazon Linux")
}

func (v *vmSuite) TestWithInstanceType() {
	instanceType := "t3.medium"
	v.UpdateEnv(awshost.Provisioner(awshost.WithoutAgent(), awshost.WithEC2InstanceOptions(ec2.WithInstanceType(instanceType))))

	vm := v.Env().RemoteHost
	metadata := client.NewEC2Metadata(vm)
	require.Equal(v.T(), instanceType, metadata.Get("instance-type"))
}

func (v *vmSuite) TestWithArch() {
	v.UpdateEnv(awshost.Provisioner(awshost.WithoutAgent(), awshost.WithEC2InstanceOptions(ec2.WithOSArch(os.DebianDefault, os.ARM64Arch))))
	require.Equal(v.T(), "aarch64\n", v.Env().RemoteHost.MustExecute("uname -m"))
}

func (v *vmSuite) TestWithUserdata() {
	path := "/tmp/test-userdata"
	v.UpdateEnv(awshost.Provisioner(awshost.WithoutAgent(), awshost.WithEC2InstanceOptions(ec2.WithUserData("#!/bin/bash\ntouch "+path))))

	output, err := v.Env().RemoteHost.Execute("ls " + path)
	require.NoError(v.T(), err)
	require.Equal(v.T(), path+"\n", output)
}
