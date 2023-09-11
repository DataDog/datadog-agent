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
	"github.com/stretchr/testify/require"
)

type vmSuiteCopy struct {
	e2e.Suite[e2e.VMEnv]
}

func TestVMSuiteCopy(t *testing.T) {
	e2e.Run(t, &vmSuiteCopy{}, e2e.EC2VMStackDef(ec2params.WithOS(ec2os.UbuntuOS)))
}

func (v *vmSuiteCopy) TestCopy() {

	v.Env().VM.CopyFolder("test-folder", "test")
	v.Env().VM.CopyFile("test-folder/file-0", "copied-file")

	output0 := v.Env().VM.Execute("cat test/file-0")
	output1 := v.Env().VM.Execute("cat test/folder-1/file-1")
	output2 := v.Env().VM.Execute("cat copied-file")

	require.Equal(v.T(), "This is a test file 0\n", output0)
	require.Equal(v.T(), "This is a test file 1\n", output1)
	require.Equal(v.T(), "This is a test file 0\n", output2)
}
