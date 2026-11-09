// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package tracer

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

func TestFindBPFNATDataplane(t *testing.T) {
	pinned := func(paths ...string) fstest.MapFS {
		fsys := fstest.MapFS{}
		for _, p := range paths {
			fsys[p] = &fstest.MapFile{}
		}
		return fsys
	}
	tests := []struct {
		name string
		fsys fstest.MapFS
		want string
	}{
		{"empty bpffs", fstest.MapFS{}, ""},
		{"unrelated pinned maps", pinned("tc/globals/my_map", "tc/globals/cilium_calls_hostns", "other/cali_x"), ""},
		{"cilium IPv4 conntrack", pinned("tc/globals/cilium_ct4_global"), "tc/globals/cilium_ct4_global"},
		{"cilium IPv6 conntrack only", pinned("tc/globals/cilium_ct6_global"), "tc/globals/cilium_ct6_global"},
		{"cilium IPv6 SNAT only", pinned("tc/globals/cilium_snat_v6_external"), "tc/globals/cilium_snat_v6_external"},
		{"cilium LB map of another version", pinned("tc/globals/cilium_lb4_backends_v2"), "tc/globals/cilium_lb4_backends_v2"},
		{"cilium IPv6 LB only", pinned("tc/globals/cilium_lb6_services_v2"), "tc/globals/cilium_lb6_services_v2"},
		{"calico eBPF in tc globals", pinned("tc/globals/cali_v4_nat_fe3"), "tc/globals/cali_v4_nat_fe3"},
		{"calico eBPF directory", pinned("calico/bpf_nat_fe_v4"), "calico"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, findBPFNATDataplane(tt.fsys))
		})
	}
}
