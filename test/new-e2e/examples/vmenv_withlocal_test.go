// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"flag"
	"os"
	"path"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/localinfra/localvmparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/assert"
)

var runLocally = flag.Bool("runLocally", false, "run tests on a local VM")

type vmSuiteExample struct {
	e2e.Suite[e2e.VMEnv]
}

func TestVMSuiteEx(t *testing.T) {
	if *runLocally {
		homeDir, _ := os.UserHomeDir()
		e2e.Run(t, &vmSuiteExample{}, e2e.LocalVMDef(localvmparams.WithJSONFile(path.Join(homeDir, ".test_config.json"))))
	} else {
		e2e.Run(t, &vmSuiteExample{}, e2e.EC2VMStackDef(ec2params.WithOS(ec2os.WindowsOS)))
	}
}

func (v *vmSuiteExample) TestItIsWindows() {
	res := v.Env().VM.Execute("dir C:\\")
	assert.Contains(v.T(), res, "Windows")
}
