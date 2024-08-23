// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	azurehost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/azure/host/linux"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
)

type azureVMSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestVMSuite runs tests for the VM interface to ensure its implementation is correct.
func TestAzureVMSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(azurehost.ProvisionerNoAgentNoFakeIntake())}
	e2e.Run(t, &azureVMSuite{}, suiteParams...)
}

func (v *azureVMSuite) TestExecute() {
	vm := v.Env().RemoteHost

	out, err := vm.Execute("dir")
	v.Require().NoError(err)
	v.Require().NotEmpty(out)
}
