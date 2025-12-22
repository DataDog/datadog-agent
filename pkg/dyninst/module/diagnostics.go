// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"cmp"
	"slices"
	"sync"

	"github.com/google/btree"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"

	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type diagnosticTracker struct {
	name string
	mu   struct {
		sync.Mutex
		removeBuf []diagnosticItem
		btree     *btree.BTreeG[diagnosticItem]
	}
}

func newDiagnosticTracker(name string) *diagnosticTracker {
	dt := &diagnosticTracker{
		name: name,
	}
	dt.mu.btree = btree.NewG(8, diagnosticItem.less)
	return dt
}

func (e *diagnosticTracker) mark(
	runtimeID string,
	probeID string,
	probeVersion int,
) (first bool) {
	key := diagnosticKey{runtimeID: runtimeID, probeID: probeID}
	item := diagnosticItem{key: key, version: probeVersion}

	e.mu.Lock()
	defer e.mu.Unlock()

	var (
		prevVersion int
		found       bool
	)
	e.mu.btree.AscendGreaterOrEqual(
		diagnosticItem{key: key}, func(item diagnosticItem) bool {
			found = item.key.runtimeID == runtimeID && item.key.probeID == probeID
			prevVersion = item.version
			return false
		},
	)
	first = !found || prevVersion < probeVersion
	if first {
		log.Tracef(
			"mark %s: probeId %v (version %v) marked for runtimeId %v",
			e.name, probeID, probeVersion, runtimeID,
		)
		e.mu.btree.ReplaceOrInsert(item)
	}
	return first
}

type diagnosticsManager struct {
	uploader  DiagnosticsUploader
	received  *diagnosticTracker
	installed *diagnosticTracker
	emitted   *diagnosticTracker
	errors    *diagnosticTracker
}

func newDiagnosticsManager(uploader DiagnosticsUploader) *diagnosticsManager {
	return &diagnosticsManager{
		uploader:  uploader,
		received:  newDiagnosticTracker("received"),
		installed: newDiagnosticTracker("installed"),
		emitted:   newDiagnosticTracker("emitted"),
		errors:    newDiagnosticTracker("errors"),
	}
}

func (m *diagnosticsManager) enqueue(
	tracker *diagnosticTracker,
	runtimeID procRuntimeID,
	probe ir.ProbeIDer,
	status uploader.Status,
	exception *uploader.DiagnosticException,
) bool {
	if !tracker.mark(runtimeID.runtimeID, probe.GetID(), probe.GetVersion()) {
		return false
	}
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
	return true
}

// retain the diagnostics for the given runtime ID for the given probes. All
// other diagnostic tracking for the runtime ID for other probes are removed.
func (m *diagnosticsManager) retain(runtimeID procRuntimeID, probes []ir.ProbeDefinition) {
	m.received.retain(runtimeID.runtimeID, probes)
	m.installed.retain(runtimeID.runtimeID, probes)
	m.emitted.retain(runtimeID.runtimeID, probes)
	m.errors.retain(runtimeID.runtimeID, probes)
}

func (m *diagnosticsManager) reportReceived(runtimeID procRuntimeID, probe ir.ProbeIDer) {
	m.enqueue(m.received, runtimeID, probe, uploader.StatusReceived, nil)
}

func (m *diagnosticsManager) reportInstalled(runtimeID procRuntimeID, probe ir.ProbeIDer) {
	m.enqueue(m.installed, runtimeID, probe, uploader.StatusInstalled, nil)
}

func (m *diagnosticsManager) reportEmitting(runtimeID procRuntimeID, probe ir.ProbeIDer) {
	m.enqueue(m.emitted, runtimeID, probe, uploader.StatusEmitting, nil)
}

func (m *diagnosticsManager) reportError(
	runtimeID procRuntimeID, probe ir.ProbeIDer, e error, errType string,
) (reported bool) {
	return m.enqueue(m.errors, runtimeID, probe, uploader.StatusError, &uploader.DiagnosticException{
		Type:    errType,
		Message: e.Error(),
	})
}

func (m *diagnosticsManager) remove(runtimeID string) {
	m.received.removeByRuntimeID(runtimeID)
	m.installed.removeByRuntimeID(runtimeID)
	m.emitted.removeByRuntimeID(runtimeID)
	m.errors.removeByRuntimeID(runtimeID)
}

func (e *diagnosticTracker) removeByRuntimeID(runtimeID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	defer e.executeRemoveBufLocked()

	key := diagnosticKey{runtimeID: runtimeID}
	e.mu.btree.AscendGreaterOrEqual(
		diagnosticItem{key: key}, func(item diagnosticItem) bool {
			if item.key.runtimeID == runtimeID {
				e.mu.removeBuf = append(e.mu.removeBuf, item)
				return true
			}
			return false
		},
	)
}

type diagnosticItem struct {
	key     diagnosticKey
	version int
}

type diagnosticKey struct {
	runtimeID string
	probeID   string
}

func cmpDiagnosticKey(a, b diagnosticKey) int {
	return cmp.Or(
		cmp.Compare(a.runtimeID, b.runtimeID),
		cmp.Compare(a.probeID, b.probeID),
	)
}

func (di diagnosticItem) less(other diagnosticItem) bool {
	return cmpDiagnosticKey(di.key, other.key) < 0
}

// Retain retains the probes for the given runtime ID. All other probes for the
// runtime ID are removed.
func (e *diagnosticTracker) retain(runtimeID string, probes []ir.ProbeDefinition) {
	slices.SortFunc(probes, ir.CompareProbeIDs)
	e.mu.Lock()
	defer e.mu.Unlock()
	defer e.executeRemoveBufLocked()

	key := diagnosticKey{runtimeID: runtimeID}
	probeIdx := 0
	e.mu.btree.AscendGreaterOrEqual(diagnosticItem{key: key}, func(
		item diagnosticItem,
	) bool {
		if item.key.runtimeID != runtimeID {
			return false
		}

		// Advance through probes to find a match for this btree item.
		// Both are sorted by (ID ascending, version descending).
		for probeIdx < len(probes) {
			p := probes[probeIdx]
			c := cmp.Or(
				cmp.Compare(p.GetID(), item.key.probeID),
				cmp.Compare(item.version, p.GetVersion()),
			)
			if c > 0 { // next probe is greater, no match possible for this item
				break
			}
			probeIdx++
			if c == 0 {
				return true
			}
		}

		// No match found for this item, add to remove buffer.
		e.mu.removeBuf = append(e.mu.removeBuf, item)
		return true
	})
}

func (e *diagnosticTracker) executeRemoveBufLocked() {
	for _, item := range e.mu.removeBuf {
		e.mu.btree.Delete(item)
	}
	clear(e.mu.removeBuf)
	e.mu.removeBuf = e.mu.removeBuf[:0]
	if cap(e.mu.removeBuf) > 128 {
		e.mu.removeBuf = nil
	}
}
