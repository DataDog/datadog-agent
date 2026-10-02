// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"slices"
	"strings"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
)

func TestConnectionDNSV2ValidatesEveryFramingBoundary(t *testing.T) {
	encoder := model.NewV2DNSEncoder()
	database, offsets, err := encoder.EncodeDomainDatabase([]string{"first.example", strings.Repeat("long", 50) + ".example", "third.example"})
	if err != nil {
		t.Fatal(err)
	}
	lookups, err := encoder.EncodeMapped(map[string]*model.DNSDatabaseEntry{
		"192.0.2.1":   {NameOffsets: []int32{0, 1}},
		"192.0.2.2":   {NameOffsets: []int32{2}},
		"2001:db8::1": {NameOffsets: []int32{0}},
		"2001:db8::2": {NameOffsets: []int32{1, 2}},
	}, offsets)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateConnectionDNSV2(database, lookups); err != nil {
		t.Fatal("valid native V2 encoding rejected")
	}
	for end := 0; end < len(database); end++ {
		if ValidateConnectionDNSV2(database[:end], lookups) == nil {
			t.Fatalf("accepted domain truncation at %d", end)
		}
	}
	for end := 0; end < len(lookups); end++ {
		if ValidateConnectionDNSV2(database, lookups[:end]) == nil {
			t.Fatalf("accepted lookup truncation at %d", end)
		}
	}
	for _, mutate := range []func([]byte){
		func(b []byte) { b[0] = 255 },
		func(b []byte) { b[1], b[2] = 0, 0 },
		func(b []byte) { b[3] = 127 },
		func(b []byte) { b[4] = 127 },
		func(b []byte) { b[len(b)-1] = 0 },
	} {
		bad := slices.Clone(lookups)
		mutate(bad)
		if ValidateConnectionDNSV2(database, bad) == nil {
			t.Fatal("accepted invalid framing or domain offset")
		}
	}
	if ValidateConnectionDNSV2(append(slices.Clone(database), 0), lookups) == nil || ValidateConnectionDNSV2(database, append(slices.Clone(lookups), 0)) == nil {
		t.Fatal("accepted unparsed trailing DNS data")
	}
}
