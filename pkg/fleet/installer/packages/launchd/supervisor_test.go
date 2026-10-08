// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package launchd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testDeadline(t *testing.T) Deadline {
	t.Helper()
	return Deadline{Path: filepath.Join(t.TempDir(), "experiment-deadline")}
}

func TestDeadlineWriteThenPresent(t *testing.T) {
	d := testDeadline(t)

	present, err := d.Present()
	require.NoError(t, err)
	assert.False(t, present, "a deadline that was never written must not report present")

	require.NoError(t, d.Write(time.Hour))

	present, err = d.Present()
	require.NoError(t, err)
	assert.True(t, present)
}

func TestDeadlineClearIsIdempotent(t *testing.T) {
	d := testDeadline(t)
	require.NoError(t, d.Write(time.Hour))

	require.NoError(t, d.Clear())
	present, err := d.Present()
	require.NoError(t, err)
	assert.False(t, present)

	// Clearing an already-absent deadline must still succeed: both a deliberate stop and a
	// crash-recovery path may race to clear the same file.
	require.NoError(t, d.Clear())
}

// TestDeadlineReadIdentifiesEachWrite pins what the watcher relies on to tell its own experiment
// from a later one: Read is empty once cleared, and two writes never persist the same value.
func TestDeadlineReadIdentifiesEachWrite(t *testing.T) {
	d := testDeadline(t)

	token, err := d.Read()
	require.NoError(t, err)
	assert.Empty(t, token)

	require.NoError(t, d.Write(time.Hour))
	first, err := d.Read()
	require.NoError(t, err)
	assert.NotEmpty(t, first)
	expired, err := d.Expired(2 * time.Hour)
	require.NoError(t, err)
	assert.False(t, expired, "a freshly written deadline must parse as unexpired")

	require.NoError(t, d.Write(time.Hour))
	second, err := d.Read()
	require.NoError(t, err)
	assert.NotEqual(t, first, second)

	require.NoError(t, d.Clear())
	token, err = d.Read()
	require.NoError(t, err)
	assert.Empty(t, token)
}

func TestDeadlineExpiredCases(t *testing.T) {
	t.Run("missing file is not expired", func(t *testing.T) {
		d := testDeadline(t)
		expired, err := d.Expired(time.Minute)
		require.NoError(t, err)
		assert.False(t, expired)
	})

	t.Run("a deadline in the past is expired", func(t *testing.T) {
		d := testDeadline(t)
		require.NoError(t, d.Write(-time.Minute))
		expired, err := d.Expired(time.Hour)
		require.NoError(t, err)
		assert.True(t, expired)
	})

	t.Run("a deadline comfortably within the window is not expired", func(t *testing.T) {
		d := testDeadline(t)
		require.NoError(t, d.Write(30*time.Minute))
		expired, err := d.Expired(time.Hour)
		require.NoError(t, err)
		assert.False(t, expired)
	})

	t.Run("an unparseable deadline is expired", func(t *testing.T) {
		d := testDeadline(t)
		require.NoError(t, os.WriteFile(d.Path, []byte("not a timestamp"), 0644))
		expired, err := d.Expired(time.Hour)
		require.NoError(t, err)
		assert.True(t, expired, "a deadline this process cannot make sense of must not be trusted to bound anything")
	})

	t.Run("a deadline implausibly far in the future is expired", func(t *testing.T) {
		d := testDeadline(t)
		// Simulates the host's clock having moved backwards since the deadline was written:
		// a window of 1h that now appears 10h away is not a real, honored deadline.
		require.NoError(t, d.Write(10*time.Hour))
		expired, err := d.Expired(time.Hour)
		require.NoError(t, err)
		assert.True(t, expired)
	})
}

// TestExitObserverDeliversAlreadyExitedPids covers ArmExitObserver's documented ESRCH-as-exit
// behavior: a pid that has already exited by the time it is armed must still produce an
// ExitEvent, not an error.
func TestExitObserverDeliversAlreadyExitedPids(t *testing.T) {
	cmd := exec.Command("/usr/bin/true")
	require.NoError(t, cmd.Start())
	require.NoError(t, cmd.Wait())

	observer, events, err := ArmExitObserver([]int{cmd.Process.Pid})
	require.NoError(t, err)
	defer observer.Disarm()

	select {
	case ev := <-events:
		assert.Equal(t, cmd.Process.Pid, ev.Pid)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the already-exited pid's event")
	}
}

// TestExitObserverDeliversLiveExit covers the ordinary path: a process armed while still running,
// observed exiting via kqueue/EVFILT_PROC.
func TestExitObserverDeliversLiveExit(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "0.2")
	require.NoError(t, cmd.Start())

	observer, events, err := ArmExitObserver([]int{cmd.Process.Pid})
	require.NoError(t, err)
	defer observer.Disarm()

	select {
	case ev := <-events:
		assert.Equal(t, cmd.Process.Pid, ev.Pid)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the live process's exit event")
	}
	_ = cmd.Wait()
}

func TestExitObserverDisarmIsIdempotent(t *testing.T) {
	observer, _, err := ArmExitObserver(nil)
	require.NoError(t, err)
	require.NoError(t, observer.Disarm())
	require.NoError(t, observer.Disarm())
}
