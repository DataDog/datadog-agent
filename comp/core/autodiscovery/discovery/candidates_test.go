// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discovery

import (
	"reflect"
	"testing"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestCandidatePorts(t *testing.T) {
	exposed := []workloadmeta.ContainerPort{{Port: 9000}, {Port: 8090}, {Port: 9001}}

	tests := []struct {
		name  string
		hints []int
		want  []uint16
	}{
		{"no hints — fallback only", nil, []uint16{9000, 8090, 9001}},
		{"hint matches one exposed", []int{8090}, []uint16{8090, 9000, 9001}},
		{"hint not exposed is dropped", []int{1234}, []uint16{9000, 8090, 9001}},
		{"two hints, declared order preserved", []int{8090, 9000}, []uint16{8090, 9000, 9001}},
		{"empty exposed yields empty", nil, []uint16{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ex := exposed
			if tc.name == "empty exposed yields empty" {
				ex = nil
			}
			got := candidatePorts(tc.hints, ex)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}
