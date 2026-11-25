// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cut_test

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

func cutRun(t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	require.NoError(t, err)

	var outBuf, errBuf bytes.Buffer
	opts := []interp.RunnerOption{
		interp.StdIO(nil, &outBuf, &errBuf),
		interp.AllowedPaths([]string{dir}),
		interpoption.AllowAllCommands().(interp.RunnerOption),
	}

	runner, err := interp.New(opts...)
	require.NoError(t, err)
	defer runner.Close()

	if dir != "" {
		runner.Dir = dir
	}

	err = runner.Run(context.Background(), prog)
	exitCode := 0
	if err != nil {
		var es interp.ExitStatus
		if errors.As(err, &es) {
			exitCode = int(es)
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func cutWriteFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

func setupCutDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		cutWriteFile(t, dir, name, content)
	}
	return dir
}

// GNU: printf 'a:b:c\n' | cut -d: -f1,3-
// Output: a:c
func TestGNUCompatCutFieldBasic(t *testing.T) {
	dir := setupCutDir(t, map[string]string{"input": "a:b:c\n"})
	stdout, _, code := cutRun(t, "cut -d: -f1,3- input", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a:c\n", stdout)
}

// GNU: printf '123\n' | cut -c4
// Output: (empty line)
func TestGNUCompatCutByteSelect(t *testing.T) {
	dir := setupCutDir(t, map[string]string{"input": "123\n"})
	stdout, _, code := cutRun(t, "cut -c4 input", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\n", stdout)
}

// GNU: printf 'a:b:c\n' | cut -d: --output-delimiter=_ -f2,3
// Output: b_c
func TestGNUCompatCutOutputDelimiter(t *testing.T) {
	dir := setupCutDir(t, map[string]string{"input": "a:b:c\n"})
	stdout, _, code := cutRun(t, "cut -d: --output-delimiter=_ -f2,3 input", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "b_c\n", stdout)
}

// GNU: printf 'abc\n' | cut -s -d: -f2,3
// Output: (nothing)
func TestGNUCompatCutSuppressNoDelim(t *testing.T) {
	dir := setupCutDir(t, map[string]string{"input": "abc\n"})
	stdout, _, code := cutRun(t, "cut -s -d: -f2,3 input", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// GNU: printf ':::\n' | cut -d: -f1-3
// Output: ::
func TestGNUCompatCutEmptyFields(t *testing.T) {
	dir := setupCutDir(t, map[string]string{"input": ":::\n"})
	stdout, _, code := cutRun(t, "cut -d: -f1-3 input", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "::\n", stdout)
}

// GNU: printf 'a\nb' | cut -f1-
// Output: a\nb\n  (trailing newline added to last line)
func TestGNUCompatCutNewlineHandling(t *testing.T) {
	dir := setupCutDir(t, map[string]string{"input": "a\nb"})
	stdout, _, code := cutRun(t, "cut -f1- input", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\n", stdout)
}

// GNU: printf 'a:1\nb:2' | cut -d: -f2
// Output: 1\n2\n
func TestGNUCompatCutFieldNoTrailing(t *testing.T) {
	dir := setupCutDir(t, map[string]string{"input": "a:1\nb:2"})
	stdout, _, code := cutRun(t, "cut -d: -f2 input", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1\n2\n", stdout)
}

// GNU: printf '123456\n' | cut --complement -b3,4-4,5,2-
// Output: 1
func TestGNUCompatCutComplement(t *testing.T) {
	dir := setupCutDir(t, map[string]string{"input": "123456\n"})
	stdout, _, code := cutRun(t, "cut --complement -b3,4-4,5,2- input", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1\n", stdout)
}

// GNU: printf 'abcd\n' | cut -b1-2,3-4 --output-delimiter=:
// Output: ab:cd
func TestGNUCompatCutOutputDelimBytesAdjacent(t *testing.T) {
	dir := setupCutDir(t, map[string]string{"input": "abcd\n"})
	stdout, _, code := cutRun(t, "cut -b1-2,3-4 --output-delimiter=: input", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ab:cd\n", stdout)
}

// GNU: printf 'abc\n' | cut -b1-2,2 --output-delimiter=:
// Output: ab  (overlapping ranges merged, no extra delimiter)
func TestGNUCompatCutOutputDelimOverlap(t *testing.T) {
	dir := setupCutDir(t, map[string]string{"input": "abc\n"})
	stdout, _, code := cutRun(t, "cut -b1-2,2 --output-delimiter=: input", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ab\n", stdout)
}

// Unknown flag should produce exit 1 and an error message.
func TestGNUCompatCutRejectedFlags(t *testing.T) {
	dir := setupCutDir(t, map[string]string{"input": "a\n"})
	for _, flag := range []string{"--no-such-flag", "-Z"} {
		_, stderr, code := cutRun(t, "cut "+flag+" input", dir)
		assert.Equal(t, 1, code, "flag: %s", flag)
		assert.Contains(t, stderr, "cut:", "flag: %s", flag)
	}
}
