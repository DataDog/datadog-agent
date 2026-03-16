// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package health

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type windowsHealthSuite struct {
	baseHealthSuite
}

func TestWindowsHealthSuite(t *testing.T) {
	t.Parallel()
	suite := &windowsHealthSuite{baseHealthSuite{descriptor: os.WindowsServerDefault}}
	e2e.Run(t, suite, e2e.WithProvisioner(awshost.Provisioner(awshost.WithRunOptions(ec2.WithEC2InstanceOptions(ec2.WithOS(suite.descriptor))))))
}
