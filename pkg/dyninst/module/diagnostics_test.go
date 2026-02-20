// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
)

func TestDiagnosticTracker_Mark(t *testing.T) {
	tr := newDiagnosticTracker("test")
	require.True(t, tr.mark("rid", "p1", 1), "expected first mark to be true")
	require.False(t, tr.mark("rid", "p1", 1), "expected duplicate version to be false")
	require.True(t, tr.mark("rid", "p1", 2), "expected higher version to be true")
	require.False(t, tr.mark("rid", "p1", 1), "expected lower version to be false")
	require.False(t, tr.mark("rid", "p1", 2), "expected duplicate version to be false")
}

func TestDiagnosticsManager_ReportReceived_DedupAndVersion(t *testing.T) {
	fu := &fakeDiagsUploader{}
	m := newDiagnosticsManager(fu)
	rid := procRuntimeID{service: "svc", runtimeID: "rid"}
	p1v1 := testProbe{id: "p1", v: 1}

	m.reportReceived(rid, p1v1)
	require.Equal(t, 1, fu.len(), "expected 1 message")
	got := fu.last()
	require.NotNil(t, got)
	require.Equal(t, 1, got.Debugger.Diagnostic.ProbeVersion)

	// Duplicate version is dropped.
	m.reportReceived(rid, p1v1)
	require.Equal(t, 1, fu.len(), "expected duplicate to be dropped")

	// Higher version enqueues.
	p1v2 := testProbe{id: "p1", v: 2}
	m.reportReceived(rid, p1v2)
	require.Equal(t, 2, fu.len(), "expected version bump to enqueue")
	got = fu.last()
	require.NotNil(t, got)
	require.Equal(t, 2, got.Debugger.Diagnostic.ProbeVersion)
}

func TestDiagnosticsManager_ReportError_IncludesException(t *testing.T) {
	fu := &fakeDiagsUploader{}
	m := newDiagnosticsManager(fu)
	rid := procRuntimeID{service: "svc", runtimeID: "rid"}
	p := testProbe{id: "p1", v: 1}

	m.reportError(rid, p, errors.New("boom"), "Bad")
	require.Equal(t, 1, fu.len(), "expected 1 error message")
	msg := fu.last()
	require.NotNil(t, msg)
	require.Equal(t, uploader.StatusError, msg.Debugger.Diagnostic.Status)
	require.NotNil(t, msg.Debugger.Diagnostic.DiagnosticException)
	require.Equal(t, "Bad", msg.Debugger.Diagnostic.DiagnosticException.Type)
}

func TestDiagnosticsManager_Remove_AllowsReenqueuing(t *testing.T) {
	fu := &fakeDiagsUploader{}
	m := newDiagnosticsManager(fu)
	rid := procRuntimeID{service: "svc", runtimeID: "rid"}
	p := testProbe{id: "p1", v: 1}

	m.reportReceived(rid, p)
	require.Equal(t, 1, fu.len(), "expected 1 message before remove")
	m.remove("rid")

	// Same version should enqueue again after removal.
	m.reportReceived(rid, p)
	require.Equal(t, 2, fu.len(), "expected enqueue after remove")
}

func TestDiagnosticsStates_Summarizes(t *testing.T) {
	fu := &fakeDiagsUploader{}
	dm := newDiagnosticsManager(fu)
	mod := &Module{diagnostics: dm}
	rid := procRuntimeID{service: "svc", runtimeID: "rid"}
	p := testProbe{id: "p1", v: 1}

	dm.reportReceived(rid, p)
	dm.reportError(rid, p, errors.New("x"), "Bad")

	states := mod.DiagnosticsStates()
	rt, ok := states["rid"]
	require.True(t, ok, "missing runtime id")
	got, ok := rt["p1"]
	require.True(t, ok, "missing probe id")
	require.Contains(t, got, "received")
	require.Contains(t, got, "errors")
}

type testProbe struct {
	id string
	v  int
}

func (p testProbe) GetID() string   { return p.id }
func (p testProbe) GetVersion() int { return p.v }

var _ ir.ProbeIDer = testProbe{}

type fakeDiagsUploader struct {
	msgs []*uploader.DiagnosticMessage
}

func (f *fakeDiagsUploader) Enqueue(
	diag *uploader.DiagnosticMessage,
) error {
	f.msgs = append(f.msgs, diag)
	return nil
}

func (f *fakeDiagsUploader) len() int {
	return len(f.msgs)
}

func (f *fakeDiagsUploader) last() *uploader.DiagnosticMessage {
	if len(f.msgs) == 0 {
		return nil
	}
	return f.msgs[len(f.msgs)-1]
}

func TestDiagnosticTracker_Retain(t *testing.T) {
	t.Run("basic retention", func(t *testing.T) {
		tr := newDiagnosticTracker("test")

		// Mark several probes across multiple runtime IDs.
		tr.mark("rid1", "p1", 1)
		tr.mark("rid1", "p2", 1)
		tr.mark("rid1", "p3", 2)
		tr.mark("rid2", "p1", 1)
		tr.mark("rid2", "p4", 1)

		// Retain only p1 and p3 for rid1.
		probes := []ir.ProbeDefinition{
			testProbeDefinition{testProbe{id: "p1", v: 1}},
			testProbeDefinition{testProbe{id: "p3", v: 2}},
		}
		tr.retain("rid1", probes)

		// p1 and p3 should still be marked (return false).
		require.False(
			t, tr.mark("rid1", "p1", 1),
			"expected p1 to still be marked",
		)
		require.False(
			t, tr.mark("rid1", "p3", 2),
			"expected p3 to still be marked",
		)

		// p2 should have been removed (return true).
		require.True(
			t, tr.mark("rid1", "p2", 1),
			"expected p2 to have been removed",
		)

		// rid2 probes should be unaffected.
		require.False(
			t, tr.mark("rid2", "p1", 1),
			"expected rid2 p1 to still be marked",
		)
		require.False(
			t, tr.mark("rid2", "p4", 1),
			"expected rid2 p4 to still be marked",
		)
	})

	t.Run("empty probe list", func(t *testing.T) {
		tr := newDiagnosticTracker("test")

		// Mark several probes.
		tr.mark("rid1", "p1", 1)
		tr.mark("rid1", "p2", 2)
		tr.mark("rid1", "p3", 1)

		// Retain with empty probe list should remove all probes for rid1.
		tr.retain("rid1", []ir.ProbeDefinition{})

		// All probes should have been removed.
		require.True(
			t, tr.mark("rid1", "p1", 1),
			"expected p1 to be removed",
		)
		require.True(
			t, tr.mark("rid1", "p2", 2),
			"expected p2 to be removed",
		)
		require.True(
			t, tr.mark("rid1", "p3", 1),
			"expected p3 to be removed",
		)
	})

	t.Run("different versions", func(t *testing.T) {
		tr := newDiagnosticTracker("test")

		// Mark probes with different versions.
		tr.mark("rid1", "p1", 1)
		tr.mark("rid1", "p1", 2)
		tr.mark("rid1", "p2", 1)

		// Retain p1 with version 2 (higher version).
		probes := []ir.ProbeDefinition{
			testProbeDefinition{testProbe{id: "p1", v: 2}},
		}
		tr.retain("rid1", probes)

		// Version 2 should still be marked.
		require.False(
			t, tr.mark("rid1", "p1", 2),
			"expected p1 v2 to still be marked",
		)

		// p2 should have been removed.
		require.True(
			t, tr.mark("rid1", "p2", 1),
			"expected p2 to have been removed",
		)
	})

	t.Run("non-existent runtime", func(t *testing.T) {
		tr := newDiagnosticTracker("test")

		// Mark some probes.
		tr.mark("rid1", "p1", 1)
		tr.mark("rid1", "p2", 1)

		// Retain on a different runtime ID should not affect rid1.
		probes := []ir.ProbeDefinition{
			testProbeDefinition{testProbe{id: "p1", v: 1}},
		}
		tr.retain("rid2", probes)

		// rid1 probes should be unaffected.
		require.False(
			t, tr.mark("rid1", "p1", 1),
			"expected rid1 p1 to still be marked",
		)
		require.False(
			t, tr.mark("rid1", "p2", 1),
			"expected rid1 p2 to still be marked",
		)
	})
}

type testProbeDefinition struct {
	testProbe
}

func (p testProbeDefinition) GetTags() []string {
	return nil
}

func (p testProbeDefinition) GetKind() ir.ProbeKind {
	return ir.ProbeKind(0)
}

func (p testProbeDefinition) GetWhere() ir.Where {
	return nil
}

func (p testProbeDefinition) GetCaptureConfig() ir.CaptureConfig {
	return nil
}

func (p testProbeDefinition) GetThrottleConfig() ir.ThrottleConfig {
	return nil
}

func (p testProbeDefinition) GetTemplate() ir.TemplateDefinition {
	return nil
}

func (p testProbeDefinition) GetCaptureExpressions() []ir.CaptureExpressionDefinition {
	return nil
}

var _ ir.ProbeDefinition = testProbeDefinition{}
