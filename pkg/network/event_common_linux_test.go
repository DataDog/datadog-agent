// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package network

import (
	"testing"

	"github.com/stretchr/testify/assert"

	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
)

func TestFromTupleAndStatsLocalNATPeerOpen(t *testing.T) {
	for _, tt := range []struct {
		name  string
		flags uint8
		want  bool
	}{
		{"no flags", 0, false},
		{"side flags only", 1<<5 | 1<<6, false},
		{"marked", flagLocalNATPeerOpen, true},
		{"marked with side flags", flagLocalNATPeerOpen | 1<<5 | 1<<6, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var c ConnectionStats
			c.FromTupleAndStats(&netebpf.ConnTuple{}, &netebpf.ConnStats{Protocol_stack: netebpf.ProtocolStack{Flags: tt.flags}})
			assert.Equal(t, tt.want, c.LocalNATPeerOpen)
		})
	}
}
