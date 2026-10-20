// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package tests_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// subshellRun runs a script with the given dir as working directory and allowed path.
func subshellRun(t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return cmdSubstRunWithOpts(t, script, dir, interp.AllowedPaths([]string{dir}), interpoption.AllowAllCommands().(interp.RunnerOption))
}

func subshellRunCtx(ctx context.Context, t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return cmdSubstRunCtxWithOpts(ctx, t, script, dir, interp.AllowedPaths([]string{dir}), interpoption.AllowAllCommands().(interp.RunnerOption))
}

// --- Basic subshell ---

func TestSubshellBasic(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := subshellRun(t, `(echo hello)`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

func TestSubshellMultipleCommands(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := subshellRun(t, `(echo hello; echo world)`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\nworld\n", stdout)
}

// --- Variable isolation ---

func TestSubshellVariableIsolation(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := subshellRun(t, `x=before; (x=inside; echo "$x"); echo "$x"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "inside\nbefore\n", stdout)
}

func TestSubshellNewVariableNotVisibleInParent(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := subshellRun(t, `(y=subshell_only); echo "[$y]"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "[]\n", stdout)
}

// --- Exit status ---

func TestSubshellExitStatus(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := subshellRun(t, `(exit 42); echo "$?"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "42\n", stdout)
}

func TestSubshellExitDoesNotExitParent(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := subshellRun(t, `(exit 1); echo "still running"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "still running\n", stdout)
}

func TestSubshellFalseExitStatus(t *testing.T) {
	dir := t.TempDir()
	_, _, code := subshellRun(t, `(false)`, dir)
	assert.Equal(t, 1, code)
}

// --- Nesting ---

func TestSubshellNested(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := subshellRun(t, `(echo outer; (echo inner))`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "outer\ninner\n", stdout)
}

func TestSubshellNestedVariableIsolation(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := subshellRun(t, `x=a; (x=b; (x=c; echo "$x"); echo "$x"); echo "$x"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "c\nb\na\n", stdout)
}

// --- Integration with pipes ---

func TestSubshellPipe(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := subshellRun(t, `(echo hello; echo world) | grep world`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "world\n", stdout)
}

// --- Integration with && and || ---

func TestSubshellAndOr(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := subshellRun(t, `(true) && echo "and works"; (false) || echo "or works"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "and works\nor works\n", stdout)
}

// --- Negation ---

func TestSubshellNegation(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := subshellRun(t, `! (false); echo "$?"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "0\n", stdout)
}

// --- Context cancellation ---

func TestSubshellContextCancellation(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stdout, _, code := subshellRunCtx(ctx, t, `(echo fast)`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "fast\n", stdout)
}

// --- Subshell with command substitution ---

func TestSubshellWithCmdSubst(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := subshellRun(t, `(x=$(echo hello); echo "$x")`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

// --- If inside subshell ---

func TestSubshellWithIf(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := subshellRun(t, `(if true; then echo yes; fi)`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "yes\n", stdout)
}

// --- For loop inside subshell ---

func TestSubshellWithForLoop(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := subshellRun(t, `(for x in a b c; do echo "$x"; done)`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\nc\n", stdout)
}
