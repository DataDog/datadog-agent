// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"

	"github.com/stretchr/testify/assert"
)

type vmSuiteEx2 struct {
	e2e.Suite[e2e.VMEnv]
}

func TestVMSuiteEx2(t *testing.T) {
	e2e.Run(t, &vmSuiteEx2{}, e2e.EC2VMStackDef(ec2params.WithImageName("ami-05fab674de2157a80", os.ARM64Arch, ec2os.AmazonLinuxOS), ec2params.WithInstanceType("c6g.medium")))
}

func (v *vmSuiteEx2) TestAmiMatch() {
	ec2Metadata := client.NewEC2Metadata(v.Env().VM)
	amiID := ec2Metadata.Get("ami-id")
	assert.Equal(v.T(), amiID, "ami-05fab674de2157a80")
}
