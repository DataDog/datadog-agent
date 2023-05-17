// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
)

type vmSuite struct {
	e2e.Suite[e2e.VMEnv]
}

func TestVMSuite(t *testing.T) {
	e2e.Run(t, &vmSuite{}, e2e.EC2VMStackDef())
}

func (v *vmSuite) TestBasicVM() {
	v.Env().VM.Execute("ls")
}
