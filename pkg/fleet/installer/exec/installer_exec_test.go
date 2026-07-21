// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package exec

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"testing"

	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRun_StructuredJSONErrorTakesPriorityOverCrashDetection guards against the ordering bug
// flagged in https://github.com/DataDog/datadog-agent/pull/53905#discussion_r3620835385: two of
// our diagnostic signatures ("cannot allocate memory", "VirtualAlloc") are plain, un-anchored
// substring matches. A structured installer error (see installerErrors.FromJSON) whose message
// happens to mention one of those phrases for an unrelated reason (e.g. a precondition check
// describing a resource constraint) must NOT be misclassified as a Go-runtime crash: doing so
// would replace the real, structured error/code with the generic ErrResourceExhausted sentinel
// and silently drop the actual diagnostic. Run() must check "is this valid JSON" before running
// isResourceExhaustionCrash, since a genuine runtime crash never emits valid JSON.
func TestRun_StructuredJSONErrorTakesPriorityOverCrashDetection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test spawns a POSIX shell command to produce subprocess stderr output")
	}

	const structuredMessage = "disk precondition check failed: cannot allocate memory for temp scratch space"
	jsonBody := fmt.Sprintf(`{"error":%q,"code":4}`, structuredMessage)

	// Sanity check that this fixture would in fact be misclassified if crash-detection ran first,
	// so this test actually exercises the ordering fix rather than a signature that never matched.
	require.True(t, isResourceExhaustionCrash([]byte(jsonBody)))

	cmd := exec.Command("sh", "-c", fmt.Sprintf("printf '%%s' %q >&2; exit 1", jsonBody))
	iCmd := &installerCmd{Cmd: cmd}

	err := iCmd.Run()

	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrResourceExhausted),
		"a structured installer error must not be misclassified as a resource-exhaustion crash just because its message contains a diagnostic substring")
	assert.Contains(t, err.Error(), structuredMessage)
}

// TestRun_GenuineCrashStillDetectedAsResourceExhaustion is the companion regression test to the
// one above: it confirms that reordering the checks in Run() to prioritize structured JSON did
// not regress detection of a genuine (non-JSON) Go-runtime crash.
func TestRun_GenuineCrashStillDetectedAsResourceExhaustion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test spawns a POSIX shell command to produce subprocess stderr output")
	}

	crashOutput := "fatal error: pageAlloc: out of memory\n\ngoroutine 1 [running]:\n"
	cmd := exec.Command("sh", "-c", fmt.Sprintf("printf '%%s' %q >&2; exit 2", crashOutput))
	iCmd := &installerCmd{Cmd: cmd}

	err := iCmd.Run()

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrResourceExhausted))
}

// TestFromJSON_NeverSignalsParseFailure documents the FromJSON behavior this fix relies on:
// FromJSON always returns a non-nil *InstallerError, even for non-JSON input (it silently falls
// back to wrapping the raw string under an "unknown" error code). This is why Run() uses
// encoding/json.Valid as an independent "is this actually structured" signal instead of trying to
// infer parse success/failure from FromJSON's return value.
func TestFromJSON_NeverSignalsParseFailure(t *testing.T) {
	err := installerErrors.FromJSON("not json at all")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "not json at all")
}
