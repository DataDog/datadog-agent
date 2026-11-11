// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package strings_cmd_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// Campaign: vuln-hunt/2026-05-19-codex. These are public-safe
// blocked-attack regressions for strings_cmd.

func TestVulnHuntBuiltinIntegerOverflow_MinLenBounds(t *testing.T) {
	dir := t.TempDir()
	writeFileBytes(t, dir, "data.bin", []byte("secret_data\n"))

	for _, script := range []string{
		"strings -n -1 data.bin",
		"strings --bytes=-1 data.bin",
		"strings -n 2147483648 data.bin",
	} {
		stdout, stderr, code := runStrings(t, script, dir)
		assert.Equal(t, 1, code, script)
		assert.Empty(t, stdout, script)
		assert.Contains(t, stderr, "strings:", script)
	}
}

func TestVulnHuntBuiltinResourceExhaustion_OutputLimitStopsSeparatorAmplification(t *testing.T) {
	dir := t.TempDir()
	var input bytes.Buffer
	for range 1200 {
		input.WriteString("a")
		input.WriteByte(0)
	}
	writeFileBytes(t, dir, "input.bin", input.Bytes())

	separator := strings.Repeat("x", 10_000)
	script := "strings -n 1 -s '" + separator + "' input.bin"
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, &stdout, &stderr),
		interpoption.AllowAllCommands().(interp.RunnerOption),
		interp.AllowedPaths([]string{dir}),
	)
	require.NoError(t, err)
	defer runner.Close()
	runner.Dir = dir

	err = runner.Run(context.Background(), prog)
	require.Error(t, err)
	assert.True(t, errors.Is(err, interp.ErrOutputLimitExceeded), "got %v", err)
	assert.LessOrEqual(t, stdout.Len(), 10*1024*1024)
	assert.Empty(t, stderr.String())
}

func TestVulnHuntBuiltinFileAccessBypass_DirectorySymlinkDoesNotLeakMetadata(t *testing.T) {
	allowed := t.TempDir()
	secret := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(secret, "secret_dir"), 0755))
	require.NoError(t, os.Symlink(filepath.Join(secret, "secret_dir"), filepath.Join(allowed, "dir_link")))

	stdout, stderr, code := runStrings(t, "strings dir_link", allowed)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "strings:")
	assert.NotContains(t, stderr, "is a directory")
}
