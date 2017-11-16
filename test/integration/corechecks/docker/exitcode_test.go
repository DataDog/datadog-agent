// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package docker

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func init() {
	registerComposeFile("exitcode.compose")
}

type exitCodeAssertSucceed struct {
	exit0  bool
	exit1  bool
	exit54 bool
}

var exitCodeAssertsState exitCodeAssertSucceed

func TestContainerExit(t *testing.T) {
	if exitCodeAssertsState.exit0 == false {
		exitCodeAssertsState.exit0 = sender.AssertServiceCheck(t, "docker.exit", metrics.ServiceCheckOK, "", []string{instanceTag}, "Container exitcode_exit0_1 exited with 0")
	}
	if !exitCodeAssertsState.exit1 == false {
		exitCodeAssertsState.exit1 = sender.AssertServiceCheck(t, "docker.exit", metrics.ServiceCheckOK, "", []string{instanceTag}, "Container exitcode_exit1_1 exited with 1")
	}
	if !exitCodeAssertsState.exit54 == false {
		exitCodeAssertsState.exit54 = sender.AssertServiceCheck(t, "docker.exit", metrics.ServiceCheckOK, "", []string{instanceTag}, "Container exitcode_exit54_1 exited with 54")
	}
}
