// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// fakeDispatcher records the last dispatched custom event. *probe.Probe satisfies the real
// dispatcher interface; here we only need to capture what Report hands off.
type fakeDispatcher struct {
	rule  *rules.Rule
	event *events.CustomEvent
	calls int
}

func (d *fakeDispatcher) DispatchCustomEvent(rule *rules.Rule, event *events.CustomEvent) {
	d.rule = rule
	d.event = event
	d.calls++
}

var _ dispatcher = (*fakeDispatcher)(nil)

func fakePCE() *model.ProcessCacheEntry {
	pce := model.NewProcessCacheEntry()
	pce.Process.PIDContext.Pid = 42
	pce.Process.Comm = "evil"
	pce.Process.FileEvent.PathnameStr = "/usr/bin/evil"
	pce.Process.ContainerContext.ContainerID = "abc123"
	return pce
}

func testMatches() []Match {
	return []Match{
		{Rule: "Evil_A", Namespace: "malware", Tags: []string{"trojan", "linux"}},
		{Rule: "Miner", Namespace: "crypto"},
	}
}

func TestNewBackendReporterNilEVM(t *testing.T) {
	assert.Nil(t, NewBackendReporter(nil, "v1"), "no event monitor means no backend reporter")
}

func TestBackendReporterMatchWithProcessSchema(t *testing.T) {
	d := &fakeDispatcher{}
	r := &BackendReporter{
		dispatcher:   d,
		newEvent:     model.NewFakeEvent,
		rulesVersion: "v1",
	}

	f := testExecFile()
	f.IsScript = true
	f.ProcessCacheEntry = fakePCE()

	r.Report(f, testSum, testMatches(), nil)

	require.Equal(t, 1, d.calls)
	require.NotNil(t, d.rule)
	assert.Equal(t, events.YaraMalwareRuleID, d.rule.Def.ID, "the rule ID is read into agent.rule_id by APIServer.SendEvent")

	data, err := d.event.MarshalJSON()
	require.NoError(t, err)
	s := string(data)

	// yara block
	assert.Contains(t, s, testSumHex, "sha256 hex")
	assert.Contains(t, s, "Evil_A")
	assert.Contains(t, s, "Miner")
	assert.Contains(t, s, `"rules_version":"v1"`)
	assert.Contains(t, s, `"script":true`)
	// process-activity schema from the serializer bound to the cache entry
	assert.Contains(t, s, `"process"`, "the full process context is serialized from the cache entry")
	assert.Contains(t, s, "evil", "the process comm/path is present")
}

func TestBackendReporterNilProcessCacheEntry(t *testing.T) {
	d := &fakeDispatcher{}
	r := &BackendReporter{
		dispatcher:   d,
		newEvent:     model.NewFakeEvent,
		rulesVersion: "v1",
	}

	f := testExecFile() // no ProcessCacheEntry

	assert.NotPanics(t, func() { r.Report(f, testSum, testMatches(), nil) })
	require.Equal(t, 1, d.calls, "a match is still reported without a process cache entry")

	data, err := d.event.MarshalJSON()
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, testSumHex)
	assert.Contains(t, s, "Evil_A")
	assert.NotContains(t, s, `"process"`, "no process schema without a cache entry")
}

func TestBackendReporterDegradesGracefully(t *testing.T) {
	// a nil reporter never panics (the pipeline passes nil when the probe can't build events)
	var nilReporter *BackendReporter
	assert.NotPanics(t, func() { nilReporter.Report(testExecFile(), testSum, testMatches(), nil) })

	// a reporter without a dispatcher is a no-op
	assert.NotPanics(t, func() { (&BackendReporter{}).Report(testExecFile(), testSum, testMatches(), nil) })

	// errors and no-match scans are left to the structured log reporter
	d := &fakeDispatcher{}
	r := &BackendReporter{dispatcher: d, newEvent: model.NewFakeEvent}
	r.Report(testExecFile(), testSum, nil, nil)
	r.Report(testExecFile(), testSum, testMatches(), errors.New("scan failed"))
	assert.Equal(t, 0, d.calls, "only successful matches are dispatched to the backend")
}
