// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usage

import (
	"slices"
	"strconv"
	"sync"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"google.golang.org/protobuf/proto"
)

// The properties the runtime usage adds to a component.
const (
	LastSeenRunningProperty = "LastSeenRunning"
	HasSetSuidBitProperty   = "HasSetSuidBit"
	RunningAsRootProperty   = "RunningAsRoot"
)

// Table holds the usage of one BOM/index instance, keyed by the final BOM ref of
// each component occurrence. A Table is built from an Index and the Reports
// answering it, so it knows which components a runtime observer could attribute
// at all, which is what tells an idle component from an unmeasurable one.
// A Table is read by whoever sends an SBOM and written by whoever receives a
// report, which are different goroutines, so it carries its own lock rather than
// leaving every caller to remember one.
type Table struct {
	Scan       ScanID
	Generation uint64
	IndexID    string

	mu sync.RWMutex
	// componentRefs and reportable validate the index-scoped ordinals a
	// runtime observer is allowed to return.
	componentRefs []string
	indexed       []bool
	reportable    []bool
	// anchored holds the BOM refs the index could attribute, whether or not they
	// were seen running.
	anchored map[string]struct{}
	// seen holds the usage reported for the BOM refs that were.
	seen map[string]Usage
	// revision counts the reports applied, so a sender can tell that the usage
	// changed since the payload it last built.
	revision uint64
}

// NewTable returns the table of an index, with no usage reported yet.
func NewTable(idx *Index) *Table {
	if idx == nil {
		return &Table{anchored: make(map[string]struct{}), seen: make(map[string]Usage)}
	}

	anchored := make(map[string]struct{}, len(idx.Components))
	componentRefs := make([]string, len(idx.Components))
	indexed := make([]bool, len(idx.Components))
	reportable := make([]bool, len(idx.Components))
	refCounts := make(map[string]int, len(idx.Components))
	for i := range idx.Components {
		componentRefs[i] = idx.Components[i].BOMRef
		if componentRefs[i] != "" {
			refCounts[componentRefs[i]]++
		}
	}
	for _, ref := range idx.Refs {
		if uint64(ref) >= uint64(len(idx.Components)) {
			continue
		}
		indexed[ref] = true
		if bomRef := idx.Components[ref].BOMRef; refCounts[bomRef] == 1 {
			reportable[ref] = true
			anchored[bomRef] = struct{}{}
		}
	}

	return &Table{
		Scan:          idx.Scan,
		Generation:    idx.Generation,
		IndexID:       idx.IndexID,
		componentRefs: componentRefs,
		indexed:       indexed,
		reportable:    reportable,
		anchored:      anchored,
		seen:          make(map[string]Usage),
	}
}

// ApplyResult describes whether a report was applied. InvalidRefs is populated
// when an otherwise matching report names ordinals that were not present in the
// published path table.
type ApplyResult struct {
	Applied     bool
	InvalidRefs []uint32
}

// Apply records a report against the table. Reports for another BOM/index
// instance, and malformed reports, are rejected atomically so they cannot make
// components appear idle on incomplete evidence.
func (t *Table) Apply(report *Report) ApplyResult {
	if t == nil || report == nil || t.IndexID == "" || report.Scan != t.Scan || report.Generation != t.Generation || report.IndexID != t.IndexID {
		return ApplyResult{}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	invalid := make(map[uint32]struct{})
	for _, u := range report.Usage {
		if uint64(u.Ref) >= uint64(len(t.componentRefs)) || !t.indexed[u.Ref] {
			invalid[u.Ref] = struct{}{}
		}
	}
	if len(invalid) > 0 {
		result := ApplyResult{InvalidRefs: make([]uint32, 0, len(invalid))}
		for ref := range invalid {
			result.InvalidRefs = append(result.InvalidRefs, ref)
		}
		slices.Sort(result.InvalidRefs)
		return result
	}

	t.revision++
	for _, u := range report.Usage {
		// A mapped path can still name a package whose source UID could not be
		// joined uniquely to the final BOM. Old observers report such ordinals;
		// accept the report but ignore those occurrences so valid entries survive.
		if !t.reportable[u.Ref] {
			continue
		}
		bomRef := t.componentRefs[u.Ref]
		if previous, ok := t.seen[bomRef]; ok {
			// Usage is monotonic within an index instance: a later report that missed
			// an access must not retract an earlier one.
			if u.LastSeen.Before(previous.LastSeen) {
				u.LastSeen = previous.LastSeen
			}
			u.Suid = u.Suid || previous.Suid
			u.AsRoot = u.AsRoot || previous.AsRoot
		}
		t.seen[bomRef] = u
	}
	return ApplyResult{Applied: true}
}

// Reported reports whether any usage has been reported for this index instance.
func (t *Table) Reported() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.revision > 0
}

// Revision returns the count of reports applied. A sender that recorded the
// revision of the payload it last built can tell that the usage has moved on.
func (t *Table) Revision() uint64 {
	if t == nil {
		return 0
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.revision
}

// Anchored returns the number of distinct component occurrences the index tied
// to at least one observable file.
func (t *Table) Anchored() int {
	if t == nil {
		return 0
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.anchored)
}

// Seen returns a copy of the usage reported so far, keyed by final BOM ref.
func (t *Table) Seen() map[string]Usage {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := make(map[string]Usage, len(t.seen))
	for bomRef, u := range t.seen {
		out[bomRef] = u
	}
	return out
}

// Stamp returns bom with the runtime usage properties of t applied to the
// components t names. Every component falls into one of three states:
//
//   - anchored in the index and present in a report, which carries the
//     timestamp and the flags;
//   - anchored in the index and absent from every report, which is idle and
//     carries a zero timestamp;
//   - not anchored in the index, which no observer could ever attribute, and so
//     carries no usage properties at all.
//
// The third state is the reason the table rather than the BOM decides: a
// component a scan found but could not tie to a file, a lock-file entry with no
// artifact on disk, would otherwise be reported idle on no evidence.
//
// Stamp leaves bom untouched and copies only the components it changes, so the
// stored BOM stays shared with its other readers. A nil t stamps nothing.
func (t *Table) Stamp(bom *cyclonedx_v1_4.Bom) *cyclonedx_v1_4.Bom {
	if t == nil || bom == nil || t.IndexID == "" || bom.GetSerialNumber() != t.IndexID {
		return bom
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.revision == 0 {
		return bom
	}

	// The envelope is rebuilt field by field and every component is carried over
	// by pointer, so only the components that gain a property are cloned. A
	// protobuf message cannot be copied by value: it carries a mutex.
	stamped := &cyclonedx_v1_4.Bom{
		SpecVersion:        bom.SpecVersion,
		Version:            bom.Version,
		SerialNumber:       bom.SerialNumber,
		Metadata:           bom.Metadata,
		Services:           bom.Services,
		ExternalReferences: bom.ExternalReferences,
		Dependencies:       bom.Dependencies,
		Compositions:       bom.Compositions,
		Vulnerabilities:    bom.Vulnerabilities,
		Components:         make([]*cyclonedx_v1_4.Component, len(bom.Components)),
	}

	for i, comp := range bom.Components {
		stamped.Components[i] = comp
		if comp == nil {
			continue
		}
		bomRef := comp.GetBomRef()
		if _, anchored := t.anchored[bomRef]; !anchored {
			continue
		}

		copied, ok := proto.Clone(comp).(*cyclonedx_v1_4.Component)
		if !ok {
			continue
		}
		copied.Properties = withUsage(comp.Properties, t.seen[bomRef])
		stamped.Components[i] = copied
	}

	return stamped
}

// withUsage returns properties with the three usage properties set to usage,
// replacing any already present so a re-stamp of the same BOM is idempotent.
func withUsage(properties []*cyclonedx_v1_4.Property, usage Usage) []*cyclonedx_v1_4.Property {
	lastSeen := "0"
	if !usage.LastSeen.IsZero() {
		lastSeen = strconv.FormatInt(usage.LastSeen.Unix(), 10)
	}

	out := make([]*cyclonedx_v1_4.Property, 0, len(properties)+3)
	for _, p := range properties {
		switch p.GetName() {
		case LastSeenRunningProperty, HasSetSuidBitProperty, RunningAsRootProperty:
		default:
			out = append(out, p)
		}
	}

	for _, p := range []struct{ name, value string }{
		{LastSeenRunningProperty, lastSeen},
		{HasSetSuidBitProperty, strconv.FormatBool(usage.Suid)},
		{RunningAsRootProperty, strconv.FormatBool(usage.AsRoot)},
	} {
		value := p.value
		out = append(out, &cyclonedx_v1_4.Property{Name: p.name, Value: &value})
	}

	return out
}
