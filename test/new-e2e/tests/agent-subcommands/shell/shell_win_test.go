// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package shell

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type windowsShellSuite struct {
	baseShellSuite
}

func TestWindowsShellSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &windowsShellSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(os.WindowsServerDefault))))))
}
