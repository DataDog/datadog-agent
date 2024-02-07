// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	rebootprovisionner "github.com/DataDog/datadog-agent/test/new-e2e/examples/rebootProvisionner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
)

type vmReboot struct {
	e2e.BaseSuite[environments.Host]
}

func TestVMReboot(t *testing.T) {
	e2e.Run(t, &vmReboot{}, e2e.WithProvisioner(rebootprovisionner.Provisioner()))
}

func (v *vmReboot) TestAmiMatch() {
	vm := v.Env().RemoteHost

	out := vm.MustExecute("cat /tmp/hello.txt")
	v.Require().Equal("hello", out)
}
