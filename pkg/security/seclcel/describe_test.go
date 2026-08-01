// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package seclcel

import (
	"strings"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDescribeEnv(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	described := DescribeEnv(env)

	// the declared variables, and the types they lead to. A type is named after
	// the path it describes, so secl.ConnectAddr rather than a secl.Addr shared
	// with every other address.
	assert.Contains(t, described, "process: secl.Process\n")
	assert.Contains(t, described, "\nsecl.ConnectAddr {\n")

	// CEL's own notation for each shape of type
	assert.Contains(t, described, "\tip: net.CIDR\n")
	assert.Contains(t, described, "\tancestors: list(secl.ProcessAncestors)\n")
	assert.Contains(t, described, "\targv: list(string)\n")
	assert.Contains(t, described, "\tuid: int\n")
	assert.Contains(t, described, "\tis_kworker: bool\n")
	assert.Contains(t, described, "\tfile: secl.ProcessFile\n")

	// walking from the variables must reach every generated type: one that is
	// unreachable would be a type no expression could name.
	for name := range modelShapes {
		assert.Contains(t, described, "\n"+name+" {\n", "type %q is not described", name)
	}

	// and describe nothing that is not declared
	assert.NotContains(t, described, "secl.Nonexistent")
}

func TestDescribeEnvIsStable(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	assert.Equal(t, DescribeEnv(env), DescribeEnv(env), "the description must not depend on map order")
}

// TestDescribeEnvShowsCallerDeclarations is the reason for reading the
// environment rather than the generated tables: a caller's own declarations show
// up too.
func TestDescribeEnvShowsCallerDeclarations(t *testing.T) {
	env, err := NewModelEnv(cel.Variable("my_macro", cel.ListType(cel.StringType)))
	require.NoError(t, err)

	described := DescribeEnv(env)
	assert.Contains(t, described, "my_macro: list(string)\n")

	// the header counts what was found rather than what was generated
	header := strings.SplitN(described, "\n", 2)[0]
	assert.Contains(t, header, "variables")
}
