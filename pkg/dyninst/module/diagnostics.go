// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"

	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type diagnosticTracker struct {
	name        string
	byRuntimeID sync.Map // map[string]*sync.Map[string]struct{}
}

func newDiagnosticTracker(name string) *diagnosticTracker {
	return &diagnosticTracker{
		name: name,
	}
}

func (e *diagnosticTracker) mark(runtimeID string, probeID string) (first bool) {
	var byProbeID *sync.Map
	{
		byProbeIDi, ok := e.byRuntimeID.Load(runtimeID)
		if !ok {
			byProbeIDi, _ = e.byRuntimeID.LoadOrStore(runtimeID, &sync.Map{})
		}
		byProbeID = byProbeIDi.(*sync.Map)
	}
	_, ok := byProbeID.LoadOrStore(probeID, struct{}{})
	if !ok {
		log.Tracef(
			"mark %s: probeId %v marked for runtimeId %v",
			e.name, probeID, runtimeID,
		)
	}
	return !ok
}

type diagnosticsManager struct {
	uploader  *uploader.DiagnosticsUploader
	received  *diagnosticTracker
	installed *diagnosticTracker
	emitted   *diagnosticTracker
}

func newDiagnosticsManager(uploader *uploader.DiagnosticsUploader) *diagnosticsManager {
	return &diagnosticsManager{
		uploader:  uploader,
		received:  newDiagnosticTracker("received"),
		installed: newDiagnosticTracker("installed"),
		emitted:   newDiagnosticTracker("emitted"),
	}
}

func (m *diagnosticsManager) enqueue(
	runtimeID procRuntimeID,
	probe ir.ProbeIDer,
	status uploader.Status,
	exception *uploader.DiagnosticException,
) {
	diag := uploader.Diagnostic{
		RuntimeID:           runtimeID.runtimeID,
		ProbeID:             probe.GetID(),
		Status:              status,
		ProbeVersion:        probe.GetVersion(),
		DiagnosticException: exception,
	}
	if err := m.uploader.Enqueue(uploader.NewDiagnosticMessage(runtimeID.service, diag)); err != nil {
		log.Warnf("error enqueuing %q diagnostic: %v", diag.Status, err)
	}
}

func (m *diagnosticsManager) reportReceived(runtimeID procRuntimeID, probe ir.ProbeIDer) {
	if m.received.mark(runtimeID.runtimeID, probe.GetID()) {
		m.enqueue(runtimeID, probe, uploader.StatusReceived, nil)
	}
}

func (m *diagnosticsManager) reportInstalled(runtimeID procRuntimeID, probe ir.ProbeIDer) {
	if m.installed.mark(runtimeID.runtimeID, probe.GetID()) {
		m.enqueue(runtimeID, probe, uploader.StatusInstalled, nil)
	}
}

func (m *diagnosticsManager) reportEmitting(runtimeID procRuntimeID, probe ir.ProbeIDer) {
	if m.emitted.mark(runtimeID.runtimeID, probe.GetID()) {
		m.enqueue(runtimeID, probe, uploader.StatusEmitting, nil)
	}
}

func (m *diagnosticsManager) reportError(runtimeID procRuntimeID, probe ir.ProbeIDer, e error, errType string) {
	m.enqueue(runtimeID, probe, uploader.StatusError, &uploader.DiagnosticException{
		Type:    errType,
		Message: e.Error(),
	})
}
