// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package languagedetection

import (
	"os/exec"
	"testing"

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
	cmd := exec.Command("/bin/sh")
	err := cmd.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		err := cmd.Process.Kill()
		if err != nil {
			t.Log("failed to kill pid:", cmd.Process.Pid)
		}
	})
	return cmd
}

func TestPrivilegedDetectorCaching(t *testing.T) {
	t.Run("cache entry does not exist", func(t *testing.T) {
		cmd1 := cmdWrapper{forkExecForTest(t)}
		d := NewPrivilegedLanguageDetector()
		d.DetectWithPrivileges([]languagemodels.Process{cmd1})

		binID, err := d.getBinID(cmd1)
		require.NoError(t, err)
		assert.True(t, d.binaryIDCache.Contains(binID))
	})
	t.Run("reuse existing cache entry", func(t *testing.T) {
		// Note that forkExecForTest spawns a sh process not a go process
		cmd1 := cmdWrapper{forkExecForTest(t)}
		d := NewPrivilegedLanguageDetector()

		binID, err := d.getBinID(cmd1)
		require.NoError(t, err)

		expectedLanguage := languagemodels.Language{Name: languagemodels.Go, Version: "1.20"}
		d.binaryIDCache.Add(binID, expectedLanguage)
		assert.Equal(t, expectedLanguage, d.DetectWithPrivileges([]languagemodels.Process{cmd1})[0])
	})
}

func TestGetBinID(t *testing.T) {
	cmd1, cmd2 := forkExecForTest(t), forkExecForTest(t)
	d := NewPrivilegedLanguageDetector()

	// Assert cmd1 and cmd2 are not the same processes
	assert.NotEqual(t, cmd1.Process.Pid, cmd2.Process.Pid)

	binID1, err := d.getBinID(cmdWrapper{cmd1})
	require.NoError(t, err)

	binID2, err := d.getBinID(cmdWrapper{cmd2})
	require.NoError(t, err)

	assert.Equal(t, binID1, binID2)
}
