// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package shell

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type linuxShellSuite struct {
	baseShellSuite
}

func TestLinuxShellSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &linuxShellSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}
