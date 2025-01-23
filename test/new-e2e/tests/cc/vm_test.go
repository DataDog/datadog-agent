// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cc

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

// TestVMSuite runs tests for the VM interface to ensure its implementation is correct.
func TestCc(t *testing.T) {
	flake.MarkOnLog(t, "hel+o")

	println("heo")

	t.Fail()
}

func TestCcFlaky(t *testing.T) {
	flake.MarkOnLog(t, "hel*o")

	println("hello")

	t.Fail()
}

func TestCcFlakyOk(t *testing.T) {
	flake.MarkOnLog(t, "hel*o")

	println("hello")
}
