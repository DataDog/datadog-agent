// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	gcphost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/gcp/host/linux"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
)

type gcpVMSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestGCPVMSuite runs tests for the VM interface to ensure its implementation is correct.
func TestGCPVMSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(gcphost.ProvisionerNoAgentNoFakeIntake())}
	e2e.Run(t, &gcpVMSuite{}, suiteParams...)
}

func (v *gcpVMSuite) TestExecute() {
	vm := v.Env().RemoteHost

	out, err := vm.Execute("whoami")
	v.Require().NoError(err)
	v.Require().NotEmpty(out)
}
