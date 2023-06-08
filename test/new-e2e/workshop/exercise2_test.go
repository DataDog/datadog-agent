// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workshop

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	compos "github.com/DataDog/test-infra-definitions/components/os"
	ec2vm "github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2VM"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/os"
	"github.com/stretchr/testify/assert"
)

type vmSuiteEx2 struct {
	e2e.Suite[e2e.VMEnv]
}

func TestVMSuiteEx2(t *testing.T) {
	e2e.Run(t, &vmSuiteEx2{}, e2e.EC2VMStackDef(ec2vm.WithImageName("ami-05fab674de2157a80", compos.ARM64Arch, os.AmazonLinuxOS), ec2vm.WithInstanceType("c6g.medium")))
}

func (v *vmSuiteEx2) TestAmiMatch() {
	ec2Metadata := client.NewEC2Metadata(v.Env().VM)
	amiID := ec2Metadata.Get("ami-id")
	assert.Equal(v.T(), amiID, "ami-05fab674de2157a80")
}
