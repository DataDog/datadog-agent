// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package testcmd_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func TestTestSymlink(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "target.txt", "hello")
	os.Symlink("target.txt", filepath.Join(dir, "link.txt"))

	_, _, code := runScript(t, `test -h link.txt`, dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)

	_, _, code = runScript(t, `test -L link.txt`, dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)

	_, _, code = runScript(t, `test -h target.txt`, dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 1, code)
}

func TestTestSymlinkNotRegular(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "target.txt", "hello")
	os.Symlink("target.txt", filepath.Join(dir, "link.txt"))

	_, _, code := runScript(t, `test -f link.txt`, dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
}

func TestTestExecutableFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "exec.sh")
	os.WriteFile(p, []byte("#!/bin/sh\n"), 0755)

	_, _, code := runScript(t, `test -x exec.sh`, dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
}

func TestTestNamedPipe(t *testing.T) {
	dir := t.TempDir()
	pipe := filepath.Join(dir, "mypipe")
	if err := syscall.Mkfifo(pipe, 0644); err != nil {
		t.Skip("cannot create FIFO:", err)
	}

	_, _, code := runScript(t, `test -p mypipe`, dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)

	_, _, code = runScript(t, `test -p .`, dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 1, code)
}
