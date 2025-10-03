// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rtloader contains tests for testing a shared library check
package sharedlibrary

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
)

type linuxSharedLibrarySuite struct {
	baseSharedLibrarySuite
}

func TestLinuxSharedLibraryImplementationSuite(t *testing.T) {
	t.Parallel()
	suite := &linuxSharedLibrarySuite{baseSharedLibrarySuite{
		libName:      "libdatadog-agent-example.so",
		targetFolder: "/opt/datadog-agent/embedded/lib",
	}}

	e2e.Run(t, suite, suite.getSuiteOptions(os.UbuntuDefault)...)
}
