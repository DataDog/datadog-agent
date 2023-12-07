// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsvm "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/vm"

	"github.com/stretchr/testify/assert"
)

type myVMSuite struct {
	e2e.BaseSuite[environments.VM]
}

func TestMyVMSuite(t *testing.T) {
	e2e.Run(t, &myVMSuite{}, e2e.WithProvisioner(awsvm.Provisioner(awsvm.WithoutAgent())))
}

func (v *myVMSuite) TestItIsUbuntu() {
	res := v.Env().Host.MustExecute("cat /etc/os-release")
	assert.Contains(v.T(), res, "Ubuntu")
}
