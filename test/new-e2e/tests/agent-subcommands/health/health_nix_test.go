// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package health

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

type linuxHealthSuite struct {
	baseHealthSuite
}

func TestLinuxHealthSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &linuxHealthSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}
