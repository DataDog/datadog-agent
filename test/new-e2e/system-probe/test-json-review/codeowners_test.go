// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package main

import (
	"bytes"
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

`

func TestLoadCodeowners(t *testing.T) {
	c := loadCodeownersWithReader(bytes.NewBuffer([]byte(ownersfile)))
	require.NotNil(t, c)

	expectedOwners := map[string]string{
		"pkg/network":               "@networks",
		"pkg/other":                 "@other",
		"pkg/wildcard":              "@wildcard",
		"pkg/third/one/very/nested": "@nested",
		"pkg/trailing":              "@trailing",
		"pkg/multiteam":             "@mainteam",
		"pkg/multiteam2":            "@mainteam",
	}

	require.Equal(t, expectedOwners, c.owners)
}

func TestMatchPackage(t *testing.T) {
	c := loadCodeownersWithReader(bytes.NewBuffer([]byte(ownersfile)))
	require.NotNil(t, c)

	require.Equal(t, "@networks", c.matchPackage("pkg/network"))
	require.Equal(t, "@networks", c.matchPackage("pkg/network/another/file"))
	require.Equal(t, "@networks", c.matchPackage("pkg/network/another/file/"))
	require.Equal(t, "@other", c.matchPackage("pkg/other/TestSomething"))
	require.Equal(t, "@wildcard", c.matchPackage("pkg/wildcard/nested"))
	require.Equal(t, "@trailing", c.matchPackage("pkg/trailing"))
	require.Equal(t, "@mainteam", c.matchPackage("pkg/multiteam/something"))
	require.Equal(t, "@mainteam", c.matchPackage("pkg/multiteam2/something"))
}
