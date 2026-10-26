// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRunnerGroups(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantChecks map[string]string
		wantGroups []string
	}{
		{"not set", "", nil, nil},
		{"invalid JSON is ignored", "not-json", nil, nil},
		{"groups", `{"kube":["kubernetes_state_core","orchestrator"],"kafka":["kafka_consumer"]}`,
			map[string]string{"kubernetes_state_core": "kube", "orchestrator": "kube", "kafka_consumer": "kafka"}, []string{"kafka", "kube"}},
		{"a check claimed twice stays with the first group in name order", `{"b":["x"],"a":["x","y"]}`,
			map[string]string{"x": "a", "y": "a"}, []string{"a", "b"}},
		{"unnamed group is ignored", `{"":["x"],"kube":["y"]}`, map[string]string{"y": "kube"}, []string{"kube"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkGroup, runnerGroups := parseRunnerGroups(tt.raw)
			assert.Equal(t, tt.wantChecks, checkGroup)
			assert.ElementsMatch(t, tt.wantGroups, slices.Collect(maps.Keys(runnerGroups)))
		})
	}
}
