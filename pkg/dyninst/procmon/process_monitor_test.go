// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procmon

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
)

// TestProcessMonitorCloseWhileAnalyzing tests that the process monitor can be
// closed while it is analyzing a process and no panics occur.
func TestProcessMonitorCloseWhileAnalyzing(t *testing.T) {
	analyzer := &blockingAnalyzer{
		analyzeChan: make(chan chan struct{}),
	}
	os.MkdirAll(t.TempDir(), 0o755)
	defer os.RemoveAll(t.TempDir())
	procRoot := filepath.Join(t.TempDir(), "proc")
	const pid = 10000
	{
		pidDir := filepath.Join(procRoot, strconv.Itoa(int(pid)))
		require.NoError(t, os.MkdirAll(pidDir, 0o755))
		require.NoError(t, os.Symlink(os.Args[0], filepath.Join(pidDir, "exe")))
		require.NoError(t, os.WriteFile(
			filepath.Join(pidDir, "environ"),
			[]byte(strings.Join([]string{
				ddServiceEnvVar + "=test",
				ddEnvironmentEnvVar + "=test",
				ddVersionEnvVar + "=1.0.0",
				ddDynInstEnabledEnvVar + "=true",
			}, "\x00")),
			0o644,
		))
	}
	pm, analyzeCh := newProcessMonitor(testHandler{}, procRoot, testResolver{}, analyzer)
	go pm.startAnalyzerWorker(analyzeCh)
	pm.NotifyExec(pid)
	pm.NotifyExec(pid + 1)
	req := <-analyzer.analyzeChan
	// Analysis is now blocked on req.

	// Shut down the process monitor while analysis is blocked.
	closeDone := make(chan struct{})
	go func() { pm.Close(); close(closeDone) }()

	// Make sure that the process monitor is closed.
	require.Eventually(t, func() bool {
		pm.mu.Lock()
		defer pm.mu.Unlock()
		return pm.mu.isClosed
	}, time.Second, 1*time.Millisecond)

	// Make sure that the Close call is still blocked on the analysis worker.
	select {
	case <-closeDone:
		t.Fatalf("process monitor closed unexpectedly")
	case <-time.After(10 * time.Millisecond):
	}

	req <- struct{}{} // allow analysis to complete
	<-closeDone       // ensure that the Close call returns
}

type blockingAnalyzer struct {
	analyzeChan chan chan struct{}
}

var _ executableAnalyzer = (*blockingAnalyzer)(nil)

func (a *blockingAnalyzer) checkFileKeyCache(process.FileKey) (interesting bool, known bool) {
	return false, false
}

func (a *blockingAnalyzer) isInteresting(*os.File, process.FileKey) (bool, error) {
	req := make(chan struct{})
	a.analyzeChan <- req
	<-req
	return false, nil
}
