// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestCheckCompatibilityFromConfig(t *testing.T) {
	tests := []struct {
		name             string
		include, exclude []string
		want             *types.CheckCompatibility
	}{
		{"nothing set is unrestricted", nil, nil, nil},
		{"include only", []string{"kubernetes_state_core"}, nil, &types.CheckCompatibility{Include: []string{"kubernetes_state_core"}}},
		{"exclude only", nil, []string{"kafka_consumer"}, &types.CheckCompatibility{Exclude: []string{"kafka_consumer"}}},
		{"both set: include wins, exclude dropped", []string{"kubernetes_state_core", "orchestrator"}, []string{"http_check"},
			&types.CheckCompatibility{Include: []string{"kubernetes_state_core", "orchestrator"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("experimental.clc_runner_checks_include", append([]string{}, tt.include...))
			cfg.SetInTest("experimental.clc_runner_checks_exclude", append([]string{}, tt.exclude...))
			assert.Equal(t, tt.want, checkCompatibilityFromConfig(cfg))
		})
	}
}
