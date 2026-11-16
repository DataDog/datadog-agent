// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// A remediation script that writes a literal "<child>:ro" entry into a
// read-write root must not change how later runners interpret the same policy.
func TestNestedReadOnlyRootSurvivesScriptCreatedROSuffixEntry(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "protected")
	require.NoError(t, os.Mkdir(child, 0755))
	config := filepath.Join(child, "config")
	key := filepath.Join(child, "key")
	require.NoError(t, os.WriteFile(config, []byte("config\n"), 0644))
	require.NoError(t, os.WriteFile(key, []byte("key\n"), 0644))

	opts := []interp.RunnerOption{
		interp.AllowedPaths([]string{parent + ":rw", child + ":ro"}),
		interp.WithMode(interp.ModeRemediation),
	}

	_, stderr, code := runScript(t, `echo x > 'protected:ro'`, parent, opts...)
	require.Equal(t, 0, code, stderr)
	require.FileExists(t, child+":ro")

	_, stderr, code = runScript(t, `echo changed > protected/config`, parent, opts...)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "permission denied")
	assert.Contains(t, stderr, "is not granted")
	_, stderr, code = runScript(t, `rm protected/key`, parent, opts...)
	assert.Equal(t, 1, code)
	assert.NotContains(t, stderr, "AllowedPaths: skipping")

	stdout, stderr, code := runScript(t, `cat protected/config; echo ok > parent.txt; cat parent.txt`, parent, opts...)
	assert.Equal(t, 0, code, stderr)
	assert.Equal(t, "config\nok\n", stdout)

	got, err := os.ReadFile(config)
	require.NoError(t, err)
	assert.Equal(t, "config\n", string(got))
	assert.FileExists(t, key)
}
