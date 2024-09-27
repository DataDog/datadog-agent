// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package health

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

type windowsHealthSuite struct {
	baseHealthSuite
}

func TestWindowsHealthSuite(t *testing.T) {
	t.Parallel()
	suite := &windowsHealthSuite{baseHealthSuite{descriptor: os.WindowsDefault}}
	e2e.Run(t, suite, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(suite.descriptor)),
	)))
}
