// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usageimpl

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
)

func TestProviderReportsAndDiagnosticsUseBOMRef(t *testing.T) {
	const duplicatePURL = "pkg:gem/actionpack@7.0.0"
	idx := &usage.Index{
		Scan:       "image:x",
		Generation: 1,
		IndexID:    "urn:uuid:index",
		Components: []usage.Component{
			{BOMRef: "occurrence-a", Purl: duplicatePURL, Name: "actionpack"},
			{BOMRef: "occurrence-b", Purl: duplicatePURL, Name: "actionpack"},
		},
		Hashes:             []uint64{1, 2},
		Refs:               []uint32{0, 1},
		UnmappedComponents: 3,
		HashCollisions:     4,
	}
	p := &provider{scans: make(map[usage.ScanID]*scan), subscribers: make(map[chan *usage.Index]struct{})}
	p.setIndex(idx)

	ack := p.Report(&usage.Report{
		Scan: idx.Scan, Generation: idx.Generation, IndexID: idx.IndexID,
		Usage: []usage.Usage{{Ref: 0, LastSeen: time.Unix(1700000000, 0)}},
	})
	if !ack.Applied || ack.IndexID != idx.IndexID || ack.Generation != idx.Generation {
		t.Fatalf("accepted ack = %#v", ack)
	}

	bad := p.Report(&usage.Report{
		Scan: idx.Scan, Generation: idx.Generation, IndexID: idx.IndexID,
		Usage: []usage.Usage{{Ref: 99}},
	})
	if bad.Applied || bad.IndexID != idx.IndexID {
		t.Fatalf("malformed report ack = %#v", bad)
	}
	stale := p.Report(&usage.Report{Scan: idx.Scan, Generation: idx.Generation, IndexID: "urn:uuid:old"})
	if stale.Applied || stale.IndexID != idx.IndexID {
		t.Fatalf("stale report ack = %#v", stale)
	}

	views := p.views()
	if len(views) != 1 {
		t.Fatalf("views = %d, want 1", len(views))
	}
	view := views[0]
	if view.IndexID != idx.IndexID || view.Anchored != 2 || view.UnmappedComponents != 3 || view.HashCollisions != 4 || view.RejectedReports != 2 {
		t.Errorf("diagnostics = %#v", view)
	}
	if len(view.InvalidReportRefs) != 1 || view.InvalidReportRefs[0] != 99 {
		t.Errorf("invalid refs = %v, want [99]", view.InvalidReportRefs)
	}
	if len(view.Usage) != 1 || view.Usage[0].BOMRef != "occurrence-a" || view.Usage[0].Purl != duplicatePURL {
		t.Errorf("usage diagnostics = %#v", view.Usage)
	}
}
