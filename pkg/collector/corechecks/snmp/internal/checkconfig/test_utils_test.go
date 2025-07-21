// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package checkconfig

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/profile"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExplicitRCConfig(t *testing.T) {
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
profile: profile1`)
	// language=yaml
	rawInitConfig := []byte(`use_remote_config_profiles: true`)
	client, err := MakeMockClient([]profiledefinition.ProfileDefinition{
		{
			Name: "profile1",
			Metrics: []profiledefinition.MetricsConfig{
				{Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.7.1.0",
					Name: "IAmACounter32",
				}},
			},
		},
	})
	defer profile.ResetRCProvider()
	require.NoError(t, err)

	_, err = NewCheckConfig(rawInstanceConfig, rawInitConfig, client)
	require.NoError(t, err)
	assert.True(t, client.subscribed)
}

func TestDynamicRCConfig(t *testing.T) {
	// language=yaml
	rawInstanceConfig := []byte(`ip_address: 1.2.3.4`)
	// language=yaml
	rawInitConfig := []byte(`use_remote_config_profiles: true`)
	client, err := MakeMockClient([]profiledefinition.ProfileDefinition{
		{
			Name:         "profile1",
			SysObjectIDs: []string{"1.2.3.4.*"},
			Metrics: []profiledefinition.MetricsConfig{
				{Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.7.1.0",
					Name: "IAmACounter32",
				}},
			},
		},
	})
	defer profile.ResetRCProvider()
	require.NoError(t, err)

	_, err = NewCheckConfig(rawInstanceConfig, rawInitConfig, client)
	require.NoError(t, err)
	assert.True(t, client.subscribed)
}

func TestRCConflict(t *testing.T) {
	// language=yaml
	rawInstanceConfig := []byte(`ip_address: 1.2.3.4`)
	// language=yaml
	rawInitConfig := []byte(`use_remote_config_profiles: true`)
	client, err := MakeMockClient([]profiledefinition.ProfileDefinition{
		{
			Name:         "profile1",
			SysObjectIDs: []string{"1.2.3.4.*"},
			Metrics: []profiledefinition.MetricsConfig{
				{Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.7.1.0",
					Name: "IAmACounter32",
				}},
			},
		}, {
			Name:         "profile2",
			SysObjectIDs: []string{"1.2.3.4.*"},
			Metrics: []profiledefinition.MetricsConfig{
				{Symbol: profiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.7.1.0",
					Name: "IAmACounter32",
				}},
			},
		},
	})
	defer profile.ResetRCProvider()
	require.NoError(t, err)

	_, err = NewCheckConfig(rawInstanceConfig, rawInitConfig, client)
	require.NoError(t, err)
	assert.True(t, client.subscribed)
}
