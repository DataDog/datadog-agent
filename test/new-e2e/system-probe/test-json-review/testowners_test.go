// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const ownersfile = `
# A comment
/pkg/network @networks
pkg/other/thing.go @other
/pkg/wildcard/* @wildcard
/pkg/third/one/very/nested/something*.go @nested
/pkg/trailing/ @trailing
/pkg/multiteam/* @mainteam
/pkg/multiteam/*others @otherteam
/pkg/multiteam2/ @mainteam
/pkg/multiteam2/*others @otherteam
/pkg/multiple/ @oneteam @twoteam
/test/new-e2e/system-probe/test-json-review/ @testowners_general
/test/new-e2e/system-probe/test-json-review/testowners_test.go @testowners_specific
`

func TestMatchPackage(t *testing.T) {
	c, err := newTestownersWithReader(bytes.NewBuffer([]byte(ownersfile)), "")
	require.NoError(t, err)
	require.NotNil(t, c)

	require.Equal(t, "@networks", c.matchTest(testEvent{Package: "pkg/network"}))
	require.Equal(t, "@networks", c.matchTest(testEvent{Package: "pkg/network/another/file"}))
	require.Equal(t, "@networks", c.matchTest(testEvent{Package: "pkg/network/another/file/"}))
	require.Equal(t, "@wildcard", c.matchTest(testEvent{Package: "pkg/wildcard/nested"}))
	require.Equal(t, "@mainteam", c.matchTest(testEvent{Package: "pkg/multiteam/something"}))
	require.Equal(t, "@oneteam @twoteam", c.matchTest(testEvent{Package: "pkg/multiple/something"}))
	require.Equal(t, "@mainteam", c.matchTest(testEvent{Package: "pkg/multiteam2/something"}))
}

func TestLoadSymbolMapForFile(t *testing.T) {
	c, err := newTestownersWithReader(bytes.NewBuffer([]byte(ownersfile)), "")
	require.NoError(t, err)
	require.NotNil(t, c)

	// Ensure that the cache is set to nil so that paths that cannot be read are not retried constantly
	c.loadSymbolMapForFile("does-not-exist")
	require.Nil(t, c.symtableCache["does-not-exist"])

	// Ensure that we can read our own executable
	thisTest, err := os.Executable()
	require.NoError(t, err)
	require.NoError(t, c.loadSymbolMapForFile(thisTest))
	require.NotNil(t, c.symtableCache[thisTest])
}

func TestMatchPackageWithFunction(t *testing.T) {
	thisTestExec, err := os.Executable()
	require.NoError(t, err)

	c, err := newTestownersWithReader(bytes.NewBuffer([]byte(ownersfile)), "")
	require.NoError(t, err)
	require.NotNil(t, c)

	// Get the file and function information of this test via runtime package, to avoid problems with refactors
	testEvent := testEvent{Test: "TestMatchPackageWithFunction", Package: "test/new-e2e/system-probe/test-json-review"}

	// Check that the symbols are loaded correctly
	require.NoError(t, c.loadSymbolMapForFile(thisTestExec))
	require.NotNil(t, c.symtableCache[thisTestExec])

	// Check that the file for the test is reported correctly
	require.Equal(t, "test/new-e2e/system-probe/test-json-review/testowners_test.go", c.getFileForTest(testEvent, thisTestExec))
	require.Equal(t, "@testowners_specific", c.matchTestWithBinary(testEvent, thisTestExec))

}
