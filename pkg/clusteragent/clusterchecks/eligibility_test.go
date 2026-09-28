// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/stretchr/testify/assert"
)

func TestIsEligible(t *testing.T) {
	tests := []struct {
		name      string
		compat    *types.CheckCompatibility
		checkName string
		want      bool
	}{
		{
			name:      "nil compat accepts anything",
			compat:    nil,
			checkName: "kubernetes_state_core",
			want:      true,
		},
		{
			name:      "empty compat accepts anything",
			compat:    &types.CheckCompatibility{},
			checkName: "kubernetes_state_core",
			want:      true,
		},
		{
			name:      "include claims the check",
			compat:    &types.CheckCompatibility{Include: []string{"kubernetes_state_core"}},
			checkName: "kubernetes_state_core",
			want:      true,
		},
		{
			name:      "include does not claim the check",
			compat:    &types.CheckCompatibility{Include: []string{"kubernetes_state_core"}},
			checkName: "http_check",
			want:      false,
		},
		{
			name:      "exclude refuses the check",
			compat:    &types.CheckCompatibility{Exclude: []string{"kubernetes_state_core"}},
			checkName: "kubernetes_state_core",
			want:      false,
		},
		{
			name:      "exclude does not refuse the check",
			compat:    &types.CheckCompatibility{Exclude: []string{"kubernetes_state_core"}},
			checkName: "http_check",
			want:      true,
		},
		{
			name: "exclude subtracted after include",
			compat: &types.CheckCompatibility{
				Include: []string{"kubernetes_state_core", "http_check"},
				Exclude: []string{"http_check"},
			},
			checkName: "kubernetes_state_core",
			want:      true,
		},
		{
			name: "include claims it but exclude subtracts it",
			compat: &types.CheckCompatibility{
				Include: []string{"kubernetes_state_core", "http_check"},
				Exclude: []string{"http_check"},
			},
			checkName: "http_check",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isEligible(tt.compat, tt.checkName))
		})
	}
}
