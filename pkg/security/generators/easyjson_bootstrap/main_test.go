// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const source = `package fixture

// Marked is picked up.
// easyjson:json
type Marked struct{}

// Unmarked is ignored.
type Unmarked struct{}

// Skipped is excluded even though it looks eligible.
// easyjson:skip
type Skipped struct{}

// Aliased is a non-struct, which easyjson only accepts when marked explicitly.
// easyjson:json
type Aliased map[string]interface{}

// easyjson:json
type (
	GroupedA struct{}
	GroupedB struct{}
)

/*
easyjson:json
*/
type BlockCommented struct{}
`

func TestAnnotatedTypes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.go")
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))

	pkgName, types, err := annotatedTypes(path)
	require.NoError(t, err)

	assert.Equal(t, "fixture", pkgName)
	// Sorted, because the order reaches the generated output.
	assert.Equal(t, []string{
		"Aliased",
		"BlockCommented",
		"GroupedA",
		"GroupedB",
		"Marked",
	}, types)
}

func TestGenerateRejectsUnexported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unexported.go")
	require.NoError(t, os.WriteFile(path, []byte("package fixture\n\n// easyjson:json\ntype hidden struct{}\n"), 0o600))

	err := generate(path, filepath.Join(dir, "bootstrap.go"), "example.com/fixture", "hidden_easyjson.go", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type Hidden = hidden")
}

func TestAnnotatedTypesNoneFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bare.go")
	require.NoError(t, os.WriteFile(path, []byte("package bare\n\ntype Plain struct{}\n"), 0o600))

	_, types, err := annotatedTypes(path)
	require.NoError(t, err)
	assert.Empty(t, types)
}
