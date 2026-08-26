// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usage

import (
	"testing"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestIndexProtoRoundTripPreservesIndexIdentity(t *testing.T) {
	in := &Index{
		Scan:       "image:x",
		Generation: 7,
		IndexID:    "urn:uuid:index",
		Status:     Ready,
		Components: []Component{{BOMRef: "core-only", Purl: "pkg:generic/a@1", Name: "a"}},
		Refs:       []uint32{0},
		Hashes:     []uint64{42},
		Paths:      []string{"/not-sent"},
	}
	out := IndexFromProto(IndexToProto(in))
	if out.Scan != in.Scan || out.Generation != in.Generation || out.IndexID != in.IndexID {
		t.Errorf("identity = {%q %d %q}, want {%q %d %q}", out.Scan, out.Generation, out.IndexID, in.Scan, in.Generation, in.IndexID)
	}
	if len(out.Components) != 1 || out.Components[0].Purl != in.Components[0].Purl {
		t.Fatalf("component metadata was not preserved: %#v", out.Components)
	}
	if out.Components[0].BOMRef != "" {
		t.Errorf("core-only BOM ref crossed the wire: %q", out.Components[0].BOMRef)
	}
	if !out.Components[0].Reportable {
		t.Error("component reportability did not cross the wire")
	}
	if out.Paths != nil {
		t.Errorf("paths crossed the wire: %v", out.Paths)
	}
}

func TestIndexFromLegacyProtoInfersPURLReportability(t *testing.T) {
	out := IndexFromProto(&pb.Index{Components: []*pb.Component{
		{Purl: "pkg:generic/a@1"},
		{Name: "no-purl"},
	}})
	if !out.Components[0].Reportable || out.Components[1].Reportable {
		t.Errorf("legacy reportability = %t/%t, want true/false", out.Components[0].Reportable, out.Components[1].Reportable)
	}
}

func TestIndexProtoReportsOnlyAnchoredUniqueBOMRefs(t *testing.T) {
	encoded := IndexToProto(&Index{
		Components: []Component{
			{BOMRef: "unique"},
			{BOMRef: "unanchored"},
			{BOMRef: "duplicate"},
			{BOMRef: "duplicate"},
		},
		Refs: []uint32{0, 2, 3},
	})
	want := []bool{true, false, false, false}
	for i, comp := range encoded.GetComponents() {
		if comp.GetReportable() != want[i] {
			t.Errorf("components[%d].reportable = %t, want %t", i, comp.GetReportable(), want[i])
		}
	}
}

func TestReportProtoRoundTripPreservesIndexIdentity(t *testing.T) {
	in := &Report{
		Scan:       "container:x",
		Generation: 9,
		IndexID:    "urn:uuid:index",
		Usage:      []Usage{{Ref: 3, LastSeen: time.Unix(1700000000, 0), Suid: true, AsRoot: true}},
	}
	out := ReportFromProto(ReportToProto(in))
	if out.Scan != in.Scan || out.Generation != in.Generation || out.IndexID != in.IndexID {
		t.Errorf("identity = {%q %d %q}, want {%q %d %q}", out.Scan, out.Generation, out.IndexID, in.Scan, in.Generation, in.IndexID)
	}
	if len(out.Usage) != 1 || out.Usage[0] != in.Usage[0] {
		t.Errorf("usage = %#v, want %#v", out.Usage, in.Usage)
	}
}
