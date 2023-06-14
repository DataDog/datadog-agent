// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package module

import (
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/rconfig"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCWSConsumer_gatherPolicyProviders(t *testing.T) {
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
			wantType: rconfig.PolicyProviderType,
			wantLen:  2,
		},
		{
			name:     "RC disabled",
			fields:   fields{config: &config.RuntimeSecurityConfig{RemoteConfigurationEnabled: false}},
			wantType: rules.PolicyProviderType,
			wantLen:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &CWSConsumer{
				config: tt.fields.config,
			}

			got := c.gatherPolicyProviders()

			assert.Equal(t, tt.wantLen, len(got))
			assert.Equal(t, tt.wantType, got[0].Type())
		})
	}
}
