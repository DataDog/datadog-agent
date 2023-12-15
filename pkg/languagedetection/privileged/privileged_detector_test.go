// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && cgo

package privileged

import (
	"os/exec"
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

func forkExecForTest(t *testing.T) *exec.Cmd {
	cmd := exec.Command("sleep", "20")
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
		cmd1 := cmdWrapper{forkExecForTest(t)}
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
		cmd1 := cmdWrapper{forkExecForTest(t)}
		d := NewLanguageDetector()

		binID, err := d.getBinID(cmd1)
		require.NoError(t, err)

		expectedLanguage := languagemodels.Language{Name: languagemodels.Go, Version: "1.20"}
		d.binaryIDCache.Add(binID, expectedLanguage)
		assert.Equal(t, expectedLanguage, d.DetectWithPrivileges([]languagemodels.Process{cmd1})[0])
	})
}

func TestGetBinID(t *testing.T) {
	cmd1, cmd2 := forkExecForTest(t), forkExecForTest(t)
	d := NewLanguageDetector()

	// Assert cmd1 and cmd2 are not the same processes
	assert.NotEqual(t, cmd1.Process.Pid, cmd2.Process.Pid)

	binID1, err := d.getBinID(cmdWrapper{cmd1})
	require.NoError(t, err)

	binID2, err := d.getBinID(cmdWrapper{cmd2})
	require.NoError(t, err)

	assert.Equal(t, binID1, binID2)
}
