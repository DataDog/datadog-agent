// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

func Test_loadInitConfigProfiles_legacyProfiles(t *testing.T) {
	SetConfdPathAndCleanProfiles()

	tests := []struct {
		name                      string
		metrics                   []profiledefinition.MetricsConfig
		expectedHaveLegacyProfile bool
	}{
		{
			name: "ok profile",
			metrics: []profiledefinition.MetricsConfig{
				{
					Name: "fooName",
				},
				{
					MIB:  "FOO-MIB",
					OID:  "1.2.3.4",
					Name: "fooName",
				},
				{
					MIB: "FOO-MIB",
					Symbol: profiledefinition.SymbolConfig{
						OID:  "1.2.3.4",
						Name: "fooName",
					},
				},
				{
					MIB: "FOO-MIB",
					Table: profiledefinition.SymbolConfig{
						OID:  "1.2.3.4",
						Name: "fooTable",
					},
					Symbols: []profiledefinition.SymbolConfig{
						{
							OID:  "1.2.3.4.1",
							Name: "fooName1",
						},
						{
							OID:  "1.2.3.4.2",
							Name: "fooName2",
						},
					},
				},
			},
			expectedHaveLegacyProfile: false,
		},
		{
			name: "legacy profile because no OID",
			metrics: []profiledefinition.MetricsConfig{
				{
					MIB:  "FOO-MIB",
					Name: "fooName",
				},
			},
			expectedHaveLegacyProfile: true,
		},
		{
			name: "legacy profile because no Symbol.OID",
			metrics: []profiledefinition.MetricsConfig{
				{
					MIB: "FOO-MIB",
					Symbol: profiledefinition.SymbolConfig{
						Name: "fooName",
					},
				},
			},
			expectedHaveLegacyProfile: true,
		},
		{
			name: "legacy profile because no Symbols[...].OID",
			metrics: []profiledefinition.MetricsConfig{
				{
					MIB: "FOO-MIB",
					Table: profiledefinition.SymbolConfig{
						OID:  "1.2.3.4",
						Name: "fooTable",
					},
					Symbols: []profiledefinition.SymbolConfig{
						{
							OID:  "1.2.3.4.1",
							Name: "fooName1",
						},
						{
							Name: "fooName2",
						},
					},
				},
			},
			expectedHaveLegacyProfile: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, haveLegacyProfile, err := loadInitConfigProfiles(ProfileConfigMap{
				"my-init-config-profile": {
					Definition: profiledefinition.ProfileDefinition{
						Metrics: tt.metrics,
					},
				},
			})
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedHaveLegacyProfile, haveLegacyProfile)
		})
	}
}
