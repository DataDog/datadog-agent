// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cc

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
)

type ccSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestVMSuite runs tests for the VM interface to ensure its implementation is correct.
func TestCc(t *testing.T) {
	flake.MarkOnLog(t, "hel*o")

	println("heo")

	t.Fail()
	// 	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake())}

	// e2e.Run(t, &ccSuite{}, suiteParams...)
}

func TestCcFlaky(t *testing.T) {
	flake.MarkOnLog(t, "hel*o")

	println("hello")

	t.Fail()
}

func TestCcFlakyOk(t *testing.T) {
	flake.MarkOnLog(t, "hel*o")

	println("hello")
}

func (v *ccSuite) TestExecute() {
	flake.MarkOnLog(v.T(), "hel*o")

	println("hellllllo")

	// vm := v.Env().RemoteHost

	// out, err := vm.Execute("whoami")
	// v.Require().NoError(err)
	// v.Require().NotEmpty(out)
}
