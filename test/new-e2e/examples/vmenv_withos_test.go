// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/assert"
)

type vmSuiteEx1 struct {
	e2e.Suite[e2e.VMEnv]
}

func TestVMSuiteEx1(t *testing.T) {
	e2e.Run(t, &vmSuiteEx1{}, e2e.EC2VMStackDef(ec2params.WithOS(ec2os.UbuntuOS)))
}

func (v *vmSuiteEx1) TestItIsUbuntu() {
	res := v.Env().VM.Execute("cat /etc/os-release")
	assert.Contains(v.T(), res, "Ubuntu")
}
