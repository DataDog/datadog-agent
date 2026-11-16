// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

package tailers

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// waitIdle waits until all the operations requested on ws are done.
func waitIdle(t *testing.T, ws *WrappedSource) {
	t.Helper()
	ws.mu.Lock()
	last := ws.last
	ws.mu.Unlock()
	if last == nil {
		return
	}
	select {
	case <-last:
	case <-time.After(10 * time.Second):
		require.FailNow(t, "WrappedSource operations did not complete")
	}
}

// A Stop shortly following a Start must not leave the source behind.
func TestWrappedSourceStopRightAfterStart(t *testing.T) {
	logSources := sources.NewLogSources()

	const n = 5000
	wrapped := make([]*WrappedSource, 0, n)
	for i := range n {
		ws := &WrappedSource{
			Source:  sources.NewLogSource(fmt.Sprintf("src-%d", i), &config.LogsConfig{Type: config.FileType, Path: "/tmp/x.log"}),
			Sources: logSources,
		}
		require.NoError(t, ws.Start())
		ws.Stop()
		wrapped = append(wrapped, ws)
	}

	for _, ws := range wrapped {
		waitIdle(t, ws)
	}
	require.Empty(t, logSources.GetSources(), "sources left behind after Start+Stop")
}

// Restarting a stopped WrappedSource adds the source back.
func TestWrappedSourceStartStopStart(t *testing.T) {
	logSources := sources.NewLogSources()
	ws := &WrappedSource{
		Source:  sources.NewLogSource("src", &config.LogsConfig{Type: config.FileType, Path: "/tmp/x.log"}),
		Sources: logSources,
	}
	require.NoError(t, ws.Start())
	ws.Stop()
	require.NoError(t, ws.Start())

	waitIdle(t, ws)
	require.Len(t, logSources.GetSources(), 1)
}
