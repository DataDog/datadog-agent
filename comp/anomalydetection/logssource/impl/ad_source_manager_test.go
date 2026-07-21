// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logssourceimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func newContainerADSource(containerID string) *sources.LogSource {
	return sources.NewLogSource("container_collect_all", &logsconfig.LogsConfig{
		Type:       logsconfig.ContainerdType,
		Identifier: containerID,
	})
}

func isSuppressed(sp *sourceProvider, containerID string) bool {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	_, suppressed := sp.suppressedIDs[containerID]
	return suppressed
}

func TestADSourceManagerAddSourceSkipsAgentContainer(t *testing.T) {
	wmeta := newWMetaMock(t)
	wmeta.Set(runningContainer("agent-container", "gcr.io/datadoghq/datadog-agent"))

	logSources := sources.NewLogSources()
	sp := newSourceProvider(wmeta, logSources, nil)
	mgr := newADSourceManager(logSources, service.NewServices(), sp)

	mgr.AddSource(newContainerADSource("agent-container"))

	assert.Empty(t, logSources.GetSources(), "AD/CCA must not bypass the agent-internal log tap gate")
	assert.False(t, isSuppressed(sp, "agent-container"), "skipped Agent AD sources should not suppress future generic decisions")
}

func TestADSourceManagerAddSourceKeepsNonAgentContainer(t *testing.T) {
	wmeta := newWMetaMock(t)
	wmeta.Set(runningContainer("app-container", "nginx"))

	logSources := sources.NewLogSources()
	sp := newSourceProvider(wmeta, logSources, nil)
	mgr := newADSourceManager(logSources, service.NewServices(), sp)

	src := newContainerADSource("app-container")
	mgr.AddSource(src)

	got := logSources.GetSources()
	require.Len(t, got, 1)
	assert.Same(t, src, got[0])
	assert.True(t, isSuppressed(sp, "app-container"), "active AD source should still suppress generic fallback collection")
}

func TestADSourceManagerAddSourceKeepsUnknownContainer(t *testing.T) {
	wmeta := newWMetaMock(t)

	logSources := sources.NewLogSources()
	sp := newSourceProvider(wmeta, logSources, nil)
	mgr := newADSourceManager(logSources, service.NewServices(), sp)

	src := newContainerADSource("not-yet-in-workloadmeta")
	mgr.AddSource(src)

	got := logSources.GetSources()
	require.Len(t, got, 1)
	assert.Same(t, src, got[0])
}
