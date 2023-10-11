// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
)

// This test is less important now that remoteConfigProvidersFirst() exists, which enforces that the RC providers are first
func TestRuleEngineGatherPolicyProviders(t *testing.T) {
	type fields struct {
		config *config.RuntimeSecurityConfig
	}
	tests := []struct {
		name     string
		fields   fields
		wantType string
		wantLen  int
	}{
		{
			name:     "RC enabled",
			fields:   fields{config: &config.RuntimeSecurityConfig{RemoteConfigurationEnabled: true}},
			wantType: rules.PolicyProviderTypeRC,
			wantLen:  3,
		},
		{
			name:     "RC disabled",
			fields:   fields{config: &config.RuntimeSecurityConfig{RemoteConfigurationEnabled: false}},
			wantType: rules.PolicyProviderTypeDir,
			wantLen:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &RuleEngine{
				config: tt.fields.config,
			}

			got := e.gatherPolicyProviders()

			assert.Equal(t, tt.wantLen, len(got))
			assert.Equal(t, tt.wantType, got[1].Type())
		})
	}
}
