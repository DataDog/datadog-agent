// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ls

import (
	"bytes"
	"context"
	"errors"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// runLsWithReadlinkFault runs ls with ReadlinkFile failing with EIO-like
// errors. A symlink target cannot fail to read natively, so the failure has to
// be injected through the CallContext.
func runLsWithReadlinkFault(t *testing.T, args ...string) (stdout, stderr string, code uint8) {
	t.Helper()
	var out, errOut bytes.Buffer
	callCtx := &builtins.CallContext{
		Stdout:      &out,
		Stderr:      &errOut,
		Now:         time.Now(),
		PortableErr: func(err error) string { return err.Error() },
		LstatFile:   func(_ context.Context, p string) (iofs.FileInfo, error) { return os.Lstat(p) },
		StatFile:    func(_ context.Context, p string) (iofs.FileInfo, error) { return os.Stat(p) },
		ReadDir:     func(_ context.Context, p string) ([]iofs.DirEntry, error) { return os.ReadDir(p) },
		ReadlinkFile: func(_ context.Context, p string) (string, error) {
			return "", &os.PathError{Op: "readlink", Path: p, Err: errors.New("input/output error")}
		},
	}
	fs := pflag.NewFlagSet("ls", pflag.ContinueOnError)
	handler := registerFlags(fs)
	require.NoError(t, fs.Parse(args))
	res := handler(context.Background(), callCtx, fs.Args())
	return out.String(), errOut.String(), res.Code
}

func TestLsLongReadlinkFailureInDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target"), nil, 0o644))
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	stdout, stderr, code := runLsWithReadlinkFault(t, "-l", dir)
	assert.Equal(t, uint8(1), code)
	assert.Contains(t, stderr, "ls: cannot read symbolic link '"+builtins.SafeOperand(joinPath(dir, "link"))+"': ")
	assert.Contains(t, stderr, "input/output error")
	// The entry is still listed, without its target.
	assert.Contains(t, stdout, " link\n")
	assert.NotContains(t, stdout, "->")
}

func TestLsLongReadlinkFailureOnOperand(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target"), nil, 0o644))
	link := filepath.Join(dir, "link")
	if err := os.Symlink("target", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	stdout, stderr, code := runLsWithReadlinkFault(t, "-l", link)
	assert.Equal(t, uint8(1), code)
	assert.Contains(t, stderr, "ls: cannot read symbolic link '"+builtins.SafeOperand(link)+"': ")
	assert.Contains(t, stderr, "input/output error")
	assert.True(t, strings.HasSuffix(strings.TrimRight(stdout, "\n"), " "+link), "entry not listed: %q", stdout)
	assert.NotContains(t, stdout, "->")
}
