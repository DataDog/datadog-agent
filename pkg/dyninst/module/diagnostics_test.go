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
	require.True(t, contains(got, "received"))
	require.True(t, contains(got, "errors"))
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

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
