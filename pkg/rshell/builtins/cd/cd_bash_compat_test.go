// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests pin behaviours that match bash's `cd` builtin byte-for-byte
// (within the constraints of our sandbox). cd is a shell builtin in bash,
// not a GNU coreutils binary, so the reference invocation in each test
// header is `bash -c '…'` rather than `cd` directly.

// TestBashCompatCdAbsoluteThenPwd — `cd /tmp; pwd` prints "/tmp\n".
//
// bash command: bash -c 'cd "$1"; pwd' _ "$DIR"
// Expected:     "$DIR\n"
func TestBashCompatCdAbsoluteThenPwd(t *testing.T) {
	dir := canonicalTempDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "real"), 0o755))
	stdout, _, code := cdRun(t, "cd "+shPath(filepath.Join(dir, "real"))+"; pwd", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, filepath.Join(dir, "real")+"\n", stdout)
}

// TestBashCompatCdRelativeThenPwd — `cd sub; pwd` from $DIR prints "$DIR/sub\n".
//
// bash command: bash -c 'cd sub; pwd'
// Expected:     "$DIR/sub\n"
func TestBashCompatCdRelativeThenPwd(t *testing.T) {
	dir := canonicalTempDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	stdout, _, code := cdRun(t, "cd sub; pwd", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, filepath.Join(dir, "sub")+"\n", stdout)
}

// TestBashCompatCdDashPrintsNewDir — `cd a; cd b; cd -` prints the
// destination directory on its own line.
//
// bash command: bash -c 'cd a; cd b; cd -'
// Expected:     "$DIR/a\n"
func TestBashCompatCdDashPrintsNewDir(t *testing.T) {
	dir := canonicalTempDir(t)
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	require.NoError(t, os.Mkdir(a, 0o755))
	require.NoError(t, os.Mkdir(b, 0o755))
	stdout, _, code := cdRun(t, "cd "+shPath(a)+"; cd "+shPath(b)+"; cd -", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, a+"\n", stdout)
}

// TestBashCompatCdNoStdoutOnPlainSuccess — a successful `cd` with a
// directory operand emits no stdout.
//
// bash command: bash -c 'cd sub'
// Expected:     ""
func TestBashCompatCdNoStdoutOnPlainSuccess(t *testing.T) {
	dir := canonicalTempDir(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	stdout, _, code := cdRun(t, "cd sub", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// TestBashCompatCdSetsPwdAndOldpwd — after `cd a`, $PWD is the new dir
// and $OLDPWD is the old.
//
// bash command: bash -c 'echo before; cd a; echo "$PWD,$OLDPWD"'
// Expected:     "before\n$DIR/a,$DIR\n"
func TestBashCompatCdSetsPwdAndOldpwd(t *testing.T) {
	dir := canonicalTempDir(t)
	a := filepath.Join(dir, "a")
	require.NoError(t, os.Mkdir(a, 0o755))
	stdout, _, code := cdRun(t, `cd a; printf '%s|%s\n' "$PWD" "$OLDPWD"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, a+"|"+dir+"\n", stdout)
}
