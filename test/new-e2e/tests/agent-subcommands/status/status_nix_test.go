// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

type linuxStatusSuite struct {
	baseStatusSuite
}

func TestLinuxStatusSuite(t *testing.T) {
	e2e.Run(t, &linuxStatusSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}
