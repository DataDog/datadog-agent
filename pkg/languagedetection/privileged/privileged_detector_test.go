// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && cgo

package privileged

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

// cmdWrapper wraps a `*exec.Cmd` into a `languagedetection.Process` interface
type cmdWrapper struct{ *exec.Cmd }

func (p cmdWrapper) GetPid() int32 {
	return int32(p.Process.Pid)
}

// GetCommand is unused
func (p cmdWrapper) GetCommand() string {
	return ""
}

func (p cmdWrapper) GetCmdline() []string {
	return p.Args
}

func forkExecForTest(t *testing.T, timeout int) *exec.Cmd {
	cmd := exec.Command("sleep", strconv.Itoa(timeout))
	err := cmd.Start()

	time.Sleep(250 * time.Millisecond)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := cmd.Process.Kill()
		if err != nil {
			t.Log("failed to kill pid:", cmd.Process.Pid)
		}
		cmd.Process.Wait()
	})
	return cmd
}

func TestPrivilegedDetectorCaching(t *testing.T) {
	t.Run("cache entry does not exist", func(t *testing.T) {
		cmd1 := cmdWrapper{forkExecForTest(t, 20)}
		d := NewLanguageDetector()
		d.DetectWithPrivileges([]languagemodels.Process{cmd1})

		binID, err := d.getBinID(cmd1)
		require.NoError(t, err)
		assert.True(t, d.binaryIDCache.Contains(binID))
	})
	t.Run("reuse existing cache entry", func(t *testing.T) {
		/*
			The process that is spawned is not a go process, so if DetectWithPrivileges returns a go process then the cache was used.

			This was done because it was believed that stubbing out `simplelru.LRUCache` was not the best solution here because:
			- `simplelru.LRUCache` has 10 methods which all have to be stubbed out, and add unnecessary bloat to the tests.
			- `simplelru.LRUCache` is an external dependency; if the interface ever changed then the test would have to be fixed.
		*/
		cmd1 := cmdWrapper{forkExecForTest(t, 20)}
		d := NewLanguageDetector()

		binID, err := d.getBinID(cmd1)
		require.NoError(t, err)

		expectedLanguage := languagemodels.Language{Name: languagemodels.Go, Version: "1.20"}
		d.binaryIDCache.Add(binID, expectedLanguage)
		assert.Equal(t, expectedLanguage, d.DetectWithPrivileges([]languagemodels.Process{cmd1})[0])
	})
}

func TestGetBinID(t *testing.T) {
	cmd1, cmd2 := forkExecForTest(t, 20), forkExecForTest(t, 20)
	d := NewLanguageDetector()

	// Assert cmd1 and cmd2 are not the same processes
	assert.NotEqual(t, cmd1.Process.Pid, cmd2.Process.Pid)

	binID1, err := d.getBinID(cmdWrapper{cmd1})
	require.NoError(t, err)

	binID2, err := d.getBinID(cmdWrapper{cmd2})
	require.NoError(t, err)

	assert.Equal(t, binID1, binID2)
}

// copyForkExecDelete creates a scenario where the executable is not longer
// available while it is running. We make a copy of "sleep" to a temporary path,
// exec the copy, and delete it. This will result in the exe symlink looking
// something like the below (note that the "(deleted)" string is part of the
// actual content of the link), so attempting to resolve the target path will
// not work, but stating/opening the exe file will still work.
//
//	/proc/123/exe -> /tmp/foo/sleep (deleted)
func copyForkExecDelete(t *testing.T, timeout int) *exec.Cmd {
	sleepBin := filepath.Join(t.TempDir(), "sleep")
	require.NoError(t, exec.Command("cp", "/bin/sleep", sleepBin).Run())

	cmd := exec.Command(sleepBin, strconv.Itoa(timeout))
	require.NoError(t, cmd.Start())

	time.Sleep(250 * time.Millisecond)

	// cmd.Start() waits for the exec to happen so deleting the file should be
	// safe immediately. Unclear what the sleep above is for; it's copied from
	// the other tests.
	os.Remove(sleepBin)

	t.Cleanup(func() {
		err := cmd.Process.Kill()
		if err != nil {
			t.Log("failed to kill pid:", cmd.Process.Pid)
		}
		cmd.Process.Wait()
	})
	return cmd
}

func TestGetBinIDDeleted(t *testing.T) {
	cmd := copyForkExecDelete(t, 20)
	d := NewLanguageDetector()
	_, err := d.getBinID(cmdWrapper{cmd})
	require.NoError(t, err)
}

func TestShortLivingProc(t *testing.T) {
	cmd := forkExecForTest(t, 1)
	_, err := cmd.Process.Wait()
	require.NoError(t, err)

	d := NewLanguageDetector()
	res := d.DetectWithPrivileges([]languagemodels.Process{cmdWrapper{cmd}})
	require.Len(t, res, 1)
	require.Equal(t, languagemodels.Language{}, res[0])
	require.Zero(t, d.binaryIDCache.Len())
}
