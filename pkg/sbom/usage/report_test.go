// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usage

import (
	"encoding/binary"
	"testing"
	"time"
)

func validReport() *Report {
	return &Report{
		Keys:          []uint64{1, 2},
		LastSeen:      []int64{10, 20},
		Flags:         []uint8{0, FlagRoot},
		ObservedSince: 5,
		KeyKind:       KeyImage,
		ImageID:       "sha256:abc",
		ContainerIDs:  []string{"c1"},
	}
}

func TestReportValidate(t *testing.T) {
	if err := validReport().Validate(); err != nil {
		t.Fatalf("Validate() of a valid report = %v", err)
	}
	for _, tc := range []struct {
		name   string
		change func(r *Report)
	}{
		{"missing times", func(r *Report) { r.LastSeen = r.LastSeen[:1] }},
		{"missing flags", func(r *Report) { r.Flags = nil }},
		{"keys out of order", func(r *Report) { r.Keys[0], r.Keys[1] = 2, 1 }},
		{"repeated key", func(r *Report) { r.Keys[1] = 1 }},
		{"never seen", func(r *Report) { r.LastSeen[0] = 0 }},
		{"unknown flag", func(r *Report) { r.Flags[0] = 1 << 7 }},
		{"negative start", func(r *Report) { r.ObservedSince = -1 }},
		{"image without an image", func(r *Report) { r.ImageID = "" }},
		{"host with an image", func(r *Report) { r.KeyKind = KeyHost }},
		{"container with an image", func(r *Report) { r.KeyKind = KeyContainer }},
		{"unknown kind", func(r *Report) { r.KeyKind = 9 }},
		{"replaced build in an image", func(r *Report) {
			r.Replaced = []Replaced{{Key: 1, ReplacedAt: 15, LastSeen: 12}}
		}},
		{"too many keys", func(r *Report) {
			r.Keys, r.LastSeen, r.Flags = make([]uint64, MaxKeys+1), make([]int64, MaxKeys+1), make([]uint8, MaxKeys+1)
		}},
	} {
		r := validReport()
		tc.change(r)
		if err := r.Validate(); err == nil {
			t.Errorf("Validate() of a report with %s = nil, want an error", tc.name)
		}
	}

	host := &Report{
		Keys:     []uint64{1},
		LastSeen: []int64{10},
		Flags:    []uint8{0},
		KeyKind:  KeyHost,
		Replaced: []Replaced{{Key: 1, ReplacedAt: 8, LastSeen: 7, StillRunning: true}, {Key: 4, ReplacedAt: 9, LastSeen: 9}},
	}
	if err := host.Validate(); err != nil {
		t.Errorf("Validate() of a host report with replaced builds = %v", err)
	}
	host.Replaced[1].Key = 1
	if err := host.Validate(); err == nil {
		t.Error("Validate() of replaced builds out of order = nil, want an error")
	}
}

// FuzzReport checks that a report from another process either fails
// validation or joins any index.
func FuzzReport(f *testing.F) {
	f.Add([]byte{}, int64(0), uint8(0), false)
	f.Add(binary.LittleEndian.AppendUint64(nil, Key("/usr/bin/bash")), int64(10), uint8(FlagRoot), true)
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8}, int64(1), uint8(3), false)

	var b Builder
	b.Add(bash, "/bin/bash", "/usr/lib/bash/libbash.so")
	b.Add(coreu, "/usr/bin/ls", "/usr/bin/env")
	b.Add(Component{BOMRef: "env"}, "/usr/bin/env")
	x := b.Build("", time.Time{}, time.Time{})

	f.Fuzz(func(t *testing.T, keys []byte, lastSeen int64, flags uint8, host bool) {
		r := &Report{KeyKind: KeyImage, ImageID: "sha256:abc"}
		if host {
			r = &Report{KeyKind: KeyHost}
		}
		for len(keys) >= 8 {
			r.Keys = append(r.Keys, binary.LittleEndian.Uint64(keys))
			r.LastSeen = append(r.LastSeen, lastSeen)
			r.Flags = append(r.Flags, flags)
			keys = keys[8:]
		}
		if len(keys) > 0 {
			r.Flags = append(r.Flags, flags)
		}
		if r.Validate() != nil {
			return
		}
		u := Join(x, r)
		if len(u) != x.Len() {
			t.Fatalf("Join = %d usages, want %d", len(u), x.Len())
		}
		for i, v := range u {
			if v.LastSeen != 0 && v.LastSeen != lastSeen || v.Flags&^flags != 0 {
				t.Errorf("Join = %+v for component %d, want last seen %d and flags within %#x", v, i, lastSeen, flags)
			}
		}
	})
}
