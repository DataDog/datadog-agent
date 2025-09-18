// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
)

type linuxCollectorSuite struct {
	baseCollectorSuite
}

func TestLinuxCollectorSuite(t *testing.T) {
	t.Parallel()
	suite := &linuxCollectorSuite{baseCollectorSuite{
		checksdPath: "/etc/datadog-agent/checks.d/multi_pid_check.py",
	}}
	e2e.Run(t, suite, suite.getSuiteOptions(os.UbuntuDefault)...)
}
