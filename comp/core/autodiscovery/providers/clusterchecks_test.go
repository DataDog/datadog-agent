// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestCheckCompatibilityFromConfig(t *testing.T) {
	tests := []struct {
		name             string
		include, exclude []string
	}{
		{"nothing set is unrestricted", nil, nil},
		{"include and exclude", []string{"kubernetes_state_core", "orchestrator"}, []string{"http_check"}},
		{"exclude only", nil, []string{"kafka_consumer"}},
		{"include only", []string{"kubernetes_state_core"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("experimental.clc_runner_checks_include", append([]string{}, tt.include...))
			cfg.SetInTest("experimental.clc_runner_checks_exclude", append([]string{}, tt.exclude...))

			compat := checkCompatibilityFromConfig(cfg)
			if len(tt.include) == 0 && len(tt.exclude) == 0 {
				assert.Nil(t, compat)
				return
			}
			require.NotNil(t, compat)
			assert.ElementsMatch(t, tt.include, compat.Include)
			assert.ElementsMatch(t, tt.exclude, compat.Exclude)
		})
	}
}
