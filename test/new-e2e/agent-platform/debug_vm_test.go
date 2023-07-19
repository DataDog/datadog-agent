// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentPlatform

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
)

type vmSuite struct {
	e2e.Suite[e2e.VMEnv]
}

func TestVMSuite(t *testing.T) {
	e2e.Run[e2e.VMEnv](t, &vmSuite{}, e2e.EC2VMStackDef(
		ec2params.WithImageName("ami-0a0c8eebcdd6dcbd0", os.ARM64Arch, ec2os.UbuntuOS),
		ec2params.WithName("My-instance"),
	))
}

func (v *vmSuite) TestBasicVM() {
	v.Env().VM.Execute("ls")
}
