// Code generated - DO NOT EDIT.
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !functionaltests

package embeddedtests

import (
	"os"
	"testing"
)

var embeddedTests = []testing.InternalTest{
	{
		Name: "TestChmod",
		F:    TestChmod,
	},
	{
		Name: "TestChown",
		F:    TestChown,
	},
	{
		Name: "TestRename",
		F:    TestRename,
	},
	{
		Name: "TestRenameInvalidate",
		F:    TestRenameInvalidate,
	},
}

func RunEmbeddedTests() {
	os.Args = []string{"embeddedtester", "-loglevel", "debug", "-test.v"}
	testing.Main(func(pat, str string) (bool, error) { return true, nil }, embeddedTests, nil, nil)
}
