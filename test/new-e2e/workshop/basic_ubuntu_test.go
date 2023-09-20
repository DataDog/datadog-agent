// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workshop

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/require"
)

type basicUbuntuSuite struct {
	e2e.Suite[e2e.VMEnv]
}

func TestBasicUbuntuSuite(t *testing.T) {
	e2e.Run(t, &basicUbuntuSuite{}, e2e.EC2VMStackDef(ec2params.WithOS(ec2os.UbuntuOS)), params.WithDevMode())
}

func (v *basicUbuntuSuite) TestBasicVM() {
	res := v.Env().VM.Execute("cat /etc/os-release")
	require.Contains(v.T(), res, "Ubuntu")
}
