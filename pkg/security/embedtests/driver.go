// Code generated - DO NOT EDIT.
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !functionaltests

package embedtests

import "testing"

var embedTests = []testing.InternalTest{
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

func RunEmbedTests() {
	testing.Main(func(pat, str string) (bool, error) { return true, nil }, embedTests, nil, nil)
}
