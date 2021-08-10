// Code generated - DO NOT EDIT.
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !functionaltests,linux

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
		Name: "TestLink",
		F:    TestLink,
	},
	{
		Name: "TestMacros",
		F:    TestMacros,
	},
	{
		Name: "TestMkdir",
		F:    TestMkdir,
	},
	{
		Name: "TestMkdirError",
		F:    TestMkdirError,
	},
	{
		Name: "TestOpen",
		F:    TestOpen,
	},
	{
		Name: "TestOpenMetadata",
		F:    TestOpenMetadata,
	},
	{
		Name: "TestRulesetLoaded",
		F:    TestRulesetLoaded,
	},
	{
		Name: "TestProcess",
		F:    TestProcess,
	},
	{
		Name: "TestProcessContext",
		F:    TestProcessContext,
	},
	{
		Name: "TestProcessExecCTime",
		F:    TestProcessExecCTime,
	},
	{
		Name: "TestProcessExec",
		F:    TestProcessExec,
	},
	{
		Name: "TestProcessMetadata",
		F:    TestProcessMetadata,
	},
	{
		Name: "TestProcessExecExit",
		F:    TestProcessExecExit,
	},
	{
		Name: "TestRename",
		F:    TestRename,
	},
	{
		Name: "TestRenameInvalidate",
		F:    TestRenameInvalidate,
	},
	{
		Name: "TestRmdir",
		F:    TestRmdir,
	},
	{
		Name: "TestRmdirInvalidate",
		F:    TestRmdirInvalidate,
	},
	{
		Name: "TestSELinux",
		F:    TestSELinux,
	},
	{
		Name: "TestSELinuxCommitBools",
		F:    TestSELinuxCommitBools,
	},
	{
		Name: "TestUnlink",
		F:    TestUnlink,
	},
	{
		Name: "TestUnlinkInvalidate",
		F:    TestUnlinkInvalidate,
	},
	{
		Name: "TestUtimes",
		F:    TestUtimes,
	},
}

func RunEmbeddedTests() {
	os.Args = []string{"embeddedtester", "-loglevel", "debug", "-test.v"}
	testing.Main(func(pat, str string) (bool, error) { return true, nil }, embeddedTests, nil, nil)
}
