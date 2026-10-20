// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sed

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callInPlace invokes sed -i's real registerFlags handler directly (rather
// than through the full shell script runner used by builtins/tests/sed),
// so a fake, error-injecting WriteRegularFile can be wired in without
// needing to actually reproduce a stalled-filesystem restore timeout
// end-to-end. The read side uses real files under dir: OpenRegularFile
// opens them for real, so only the destructive write is faked.
func callInPlace(t *testing.T, args []string, dir string, writeRegularFile func(ctx context.Context, path string, data []byte, expectedIdentity os.FileInfo) (bool, error)) (stderr string, code int) {
	t.Helper()
	fs := pflag.NewFlagSet("sed", pflag.ContinueOnError)
	fs.SetOutput(bytes.NewBuffer(nil))
	handler := registerFlags(fs)
	require.NoError(t, fs.Parse(args))

	var errBuf bytes.Buffer
	callCtx := &builtins.CallContext{
		Stdout:          bytes.NewBuffer(nil),
		Stderr:          &errBuf,
		RemediationMode: true,
		PortableErr:     func(err error) string { return err.Error() },
		AllowedPathsList: func() []builtins.AllowedPath {
			return []builtins.AllowedPath{{Path: dir, Access: builtins.AllowedPathReadWrite}}
		},
		OpenRegularFile: func(ctx context.Context, path string) (io.ReadCloser, error) {
			return os.Open(path)
		},
		WriteRegularFile: writeRegularFile,
	}

	result := handler(context.Background(), callCtx, fs.Args())
	return errBuf.String(), int(result.Code)
}

// TestInPlaceOperandLoopStopsAfterUnresolvedRestore is a regression test
// for a P1 finding: writeBack can return an error wrapping
// builtins.ErrWriteOutcomeUnknown when a primary write partially fails
// and the best-effort restore itself then times out while an abandoned
// background goroutine may still be mutating the file. The multi-file
// operand loop previously treated this exactly like any other per-file
// failure (mark failed, continue to the next operand) \u2014 but if a later
// operand names the same path, that later edit would read and overwrite
// the same inode the abandoned restore might still be writing to, racing
// it and potentially producing corrupted, nondeterministic content. The
// loop must instead stop processing every remaining operand entirely.
func TestInPlaceOperandLoopStopsAfterUnresolvedRestore(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("hello\n"), 0644))
	untouched := filepath.Join(dir, "untouched.txt")
	require.NoError(t, os.WriteFile(untouched, []byte("keep me\n"), 0644))

	unresolvedErr := fmt.Errorf("write outcome unknown, restore skipped to avoid racing the still-in-progress write: %w", builtins.ErrWriteOutcomeUnknown)
	var writeCalls int
	writeRegularFile := func(ctx context.Context, path string, data []byte, expectedIdentity os.FileInfo) (bool, error) {
		writeCalls++
		return true, unresolvedErr
	}

	stderr, code := callInPlace(t, []string{"-i", "s/hello/bye/", target, untouched}, dir, writeRegularFile)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "target.txt")

	// The write attempt itself always fails here (the fake never actually
	// writes), so untouched.txt's on-disk content staying unchanged proves
	// nothing on its own — it would stay unchanged even if the loop
	// incorrectly continued on to attempt (and fail) a second write against
	// it. What actually pins the fix is that WriteRegularFile must only
	// have been called once (for target.txt): the loop must stop entirely
	// after the unresolved restore, never even attempting to process
	// untouched.txt.
	assert.Equal(t, 1, writeCalls, "the operand loop must stop entirely after an unresolved restore, never attempting a write against a later operand")
}

// TestInPlaceOperandLoopContinuesAfterOrdinaryFailure is the control case:
// an ordinary per-file failure (not wrapping ErrWriteOutcomeUnknown) must
// still allow later operands to be processed, exactly as before this
// round's fix \u2014 only the specific unresolved-restore signal changes the
// loop's behaviour.
func TestInPlaceOperandLoopContinuesAfterOrdinaryFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("hello\n"), 0644))
	other := filepath.Join(dir, "other.txt")
	require.NoError(t, os.WriteFile(other, []byte("hello\n"), 0644))

	ordinaryErr := fmt.Errorf("no space left on device")
	writeRegularFile := func(ctx context.Context, path string, data []byte, expectedIdentity os.FileInfo) (bool, error) {
		return false, ordinaryErr
	}

	stderr, code := callInPlace(t, []string{"-i", "s/hello/bye/", target, other}, dir, writeRegularFile)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "target.txt")
	assert.Contains(t, stderr, "other.txt", "an ordinary per-file failure must not stop the loop from reaching later operands")
}
