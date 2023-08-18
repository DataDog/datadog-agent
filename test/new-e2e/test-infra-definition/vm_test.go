// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"

	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/require"
)

type vmSuite struct {
	e2e.Suite[e2e.VMEnv]
}

func TestVMSuite(t *testing.T) {
	e2e.Run(t, &vmSuite{}, e2e.EC2VMStackDef())
}

func (v *vmSuite) TestWithImageName() {
	requested_ami := "ami-05fab674de2157a80"
	v.UpdateEnv(e2e.EC2VMStackDef(
		ec2params.WithImageName(requested_ami, os.ARM64Arch, ec2os.AmazonLinuxOS)))

	vm := v.Env().VM
	metadata := client.NewEC2Metadata(vm)
	require.Equal(v.T(), requested_ami, metadata.Get("ami-id"))
	require.Equal(v.T(), "aarch64\n", vm.Execute("uname -m"))
	require.Contains(v.T(), vm.Execute("grep PRETTY_NAME /etc/os-release"), "Amazon Linux")
}

func (v *vmSuite) TestWithInstanceType() {
	instance_type := "t3.medium"
	v.UpdateEnv(e2e.EC2VMStackDef(ec2params.WithInstanceType(instance_type)))

	vm := v.Env().VM
	metadata := client.NewEC2Metadata(vm)
	require.Equal(v.T(), instance_type, metadata.Get("instance-type"))
}

func (v *vmSuite) TestWithArch() {
	v.UpdateEnv(e2e.EC2VMStackDef(ec2params.WithArch(ec2os.AmazonLinuxOS, os.ARM64Arch)))
	require.Equal(v.T(), "aarch64\n", v.Env().VM.Execute("uname -m"))
}

func (v *vmSuite) TestWithUserdata() {
	path := "/tmp/test-userdata"
	v.UpdateEnv(e2e.EC2VMStackDef(ec2params.WithOS(ec2os.AmazonLinuxOS), ec2params.WithUserData("#!/bin/bash\ntouch "+path)))

	output, err := v.Env().VM.ExecuteWithError("ls " + path)
	require.NoError(v.T(), err)
	require.Equal(v.T(), path+"\n", output)
}
