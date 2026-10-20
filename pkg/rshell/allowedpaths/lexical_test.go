// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLexicalPolicyMatchesSandboxGrants(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	require.NoError(t, os.Mkdir(child, 0700))
	for _, grants := range [][]string{
		{root}, {root + ":rw"}, {root + ":rw", child + ":ro"}, {root + ":ro", child + ":rw"},
		{root + ":rw", root + ":ro"}, {root + ":ro", root + ":rw"},
	} {
		t.Run(grants[0], func(t *testing.T) {
			lexical, err := NewLexicalPolicy(grants)
			require.NoError(t, err)
			sandbox, warnings, err := New(grants)
			require.NoError(t, err)
			require.Empty(t, warnings)
			defer sandbox.Close()
			sandbox.SetWritable()
			for _, path := range []string{"file", "child/file", "../outside", root + "-sibling/file"} {
				for _, op := range []PathOperation{PathRead, PathWrite, PathRemove, PathStat, PathLstat} {
					want := sandbox.CheckPath(path, root, op)
					got := lexical.CheckPath(path, root, op)
					assert.Equal(t, want == nil, got == nil, "%s %q with %v: sandbox %v, lexical %v", op, path, grants, want, got)
				}
			}
		})
	}
}

func TestLexicalPolicySuffixUncertainty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("literal suffix paths are a POSIX execution compatibility rule")
	}
	root := filepath.Join(t.TempDir(), "remote")
	policy, err := NewLexicalPolicy([]string{root + ":rw"})
	require.NoError(t, err)
	assert.NoError(t, policy.CheckPath(root+"/file", "", PathWrite))
	assert.NoError(t, policy.CheckPath(root+":rw/file", "", PathRead), "the remote literal root might exist")
	assert.Error(t, policy.CheckPath(root+":rw/file", "", PathWrite), "literal suffix roots never grant writes")
	assert.Error(t, policy.CheckPath(root+":rw-other/file", "", PathRead))
	assert.Error(t, policy.CheckPath("relative", "", PathRead))
	assert.Error(t, policy.CheckPath(root+"/file", "", PathOperation("invalid")))
}
