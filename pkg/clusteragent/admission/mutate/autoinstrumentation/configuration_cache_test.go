// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/stretchr/testify/require"
)

type ssiConfig struct {
	enabled            bool
	enabledNamespaces  []string
	disabledNamespaces []string
}

func TestUpdateConfiguration(t *testing.T) {
	tests := []struct {
		name           string
		localConfig    ssiConfig
		remoteConfigs  []ssiConfig
		clusterName    string
		expectedConfig ssiConfig
	}{
		{
			name: "Enable instrumentation in the cluster, local instrumentation is off",
			localConfig: ssiConfig{
				enabled: false,
			},
			remoteConfigs: []ssiConfig{{enabled: true}},
			clusterName:   "dev",
			expectedConfig: ssiConfig{
				enabled: true,
			},
		},
		{
			name: "Enable instrumentation in the namespace, local instrumentation is off",
			localConfig: ssiConfig{
				enabled: false,
			},
			remoteConfigs: []ssiConfig{
				{enabled: true, enabledNamespaces: []string{"apps"}},
			},
			clusterName: "dev",
			expectedConfig: ssiConfig{
				enabled:           true,
				enabledNamespaces: []string{"apps"},
			},
		},
		{
			name: "Enable instrumentation in the namespace, local instrumentation is on in another namespace",
			localConfig: ssiConfig{
				enabled:           true,
				enabledNamespaces: []string{"apps1"},
			},
			remoteConfigs: []ssiConfig{
				{enabled: true, enabledNamespaces: []string{"apps2"}},
			},
			clusterName: "dev",
			expectedConfig: ssiConfig{
				enabled:           true,
				enabledNamespaces: []string{"apps1", "apps2"},
			},
		},
		{
			name: "Enable instrumentation in locally disabled namespace",
			localConfig: ssiConfig{
				enabled:            true,
				disabledNamespaces: []string{"apps"},
			},
			remoteConfigs: []ssiConfig{
				{enabled: true, enabledNamespaces: []string{"apps"}},
			},
			clusterName: "dev",
			expectedConfig: ssiConfig{
				enabled:            true,
				disabledNamespaces: []string{},
			},
		},
		{
			name: "Enable instrumentation in one of locally disabled namespaces",
			localConfig: ssiConfig{
				enabled:            true,
				disabledNamespaces: []string{"apps1", "apps2"},
			},
			remoteConfigs: []ssiConfig{
				{enabled: true, enabledNamespaces: []string{"apps2"}},
			},
			clusterName: "dev",
			expectedConfig: ssiConfig{
				enabled:            true,
				disabledNamespaces: []string{"apps1"},
			},
		},
		{
			name: "Enable instrumentation in multiple namespaces in sequence, local instrumentation is off",
			localConfig: ssiConfig{
				enabled: false,
			},
			remoteConfigs: []ssiConfig{
				{enabled: true, enabledNamespaces: []string{"ns1"}},
				{enabled: true, enabledNamespaces: []string{"ns2"}},
				{enabled: true, enabledNamespaces: []string{"ns3"}},
			},
			clusterName: "dev",
			expectedConfig: ssiConfig{
				enabled:           true,
				enabledNamespaces: []string{"ns1", "ns2", "ns3"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled := tt.localConfig.enabled
			enabledNamespaces := tt.localConfig.enabledNamespaces
			disabledNamespaces := tt.localConfig.disabledNamespaces
			c := newInstrumentationConfigurationCache(&enabled, &enabledNamespaces, &disabledNamespaces, "")

			for _, remoteConfig := range tt.remoteConfigs {
				c.updateConfiguration(remoteConfig.enabled, &remoteConfig.enabledNamespaces, tt.clusterName, 1)
			}
			require.Equal(t, tt.expectedConfig.enabled, c.currentConfiguration.enabled)
			require.Equal(t, tt.expectedConfig.enabledNamespaces, c.currentConfiguration.enabledNamespaces)
			require.Equal(t, tt.expectedConfig.disabledNamespaces, c.currentConfiguration.disabledNamespaces)
		})
	}
}

func TestDeleteConfiguration(t *testing.T) {
	t1, _ := time.Parse(time.RFC822, "01 Jan 17 10:00 UTC")
	t2, _ := time.Parse(time.RFC822, "01 Apr 19 10:00 UTC")
	tnow := time.Now().UnixMilli()
	tests := []struct {
		name                     string
		localConfig              ssiConfig
		currentConfig            ssiConfig
		orderedRevisions         []int64
		enabledConfigIDs         map[string]interface{}
		enabledRevisions         map[int64]enablementConfig
		remoteConfigIDsToDisable []string
		clusterName              string
		expectedConfig           ssiConfig
	}{
		{
			name: "Delete the only remote configuration",
			localConfig: ssiConfig{
				enabled: false,
			},
			currentConfig:    ssiConfig{enabled: true},
			orderedRevisions: []int64{tnow},
			enabledConfigIDs: map[string]interface{}{
				"abc": struct{}{},
			},
			enabledRevisions: map[int64]enablementConfig{
				tnow: {configID: "abc", rcVersion: 1, rcAction: "enable", env: newString("dev"), enabled: newTrue()},
			},
			remoteConfigIDsToDisable: []string{"abc"},
			clusterName:              "dev",
			expectedConfig: ssiConfig{
				enabled: false,
			},
		},
		{
			name: "Delete last applied remote configuration",
			localConfig: ssiConfig{
				enabled: false,
			},
			currentConfig:    ssiConfig{enabled: true, enabledNamespaces: []string{"ns1", "ns2"}},
			orderedRevisions: []int64{t1.UnixMilli(), t2.UnixMilli()},
			enabledConfigIDs: map[string]interface{}{
				"abc": struct{}{},
				"def": struct{}{},
			},
			enabledRevisions: map[int64]enablementConfig{
				t1.UnixMilli(): {
					configID:          "abc",
					rcVersion:         1,
					rcAction:          "enable",
					env:               newString("dev"),
					enabled:           newTrue(),
					enabledNamespaces: &[]string{"ns1"},
				},
				t2.UnixMilli(): {
					configID:          "def",
					rcVersion:         1,
					rcAction:          "enable",
					env:               newString("dev"),
					enabled:           newTrue(),
					enabledNamespaces: &[]string{"ns2"},
				},
			},
			remoteConfigIDsToDisable: []string{"def"},
			clusterName:              "dev",
			expectedConfig: ssiConfig{
				enabled:           true,
				enabledNamespaces: []string{"ns1"},
			},
		},
		{
			name: "Delete first applied remote configuration",
			localConfig: ssiConfig{
				enabled: false,
			},
			currentConfig:    ssiConfig{enabled: true, enabledNamespaces: []string{"ns1", "ns2"}},
			orderedRevisions: []int64{t1.UnixMilli(), t2.UnixMilli()},
			enabledConfigIDs: map[string]interface{}{
				"abc": struct{}{},
				"def": struct{}{},
			},
			enabledRevisions: map[int64]enablementConfig{
				t1.UnixMilli(): {
					configID:          "abc",
					rcVersion:         1,
					rcAction:          "enable",
					env:               newString("dev"),
					enabled:           newTrue(),
					enabledNamespaces: &[]string{"ns1"},
				},
				t2.UnixMilli(): {
					configID:          "def",
					rcVersion:         1,
					rcAction:          "enable",
					env:               newString("dev"),
					enabled:           newTrue(),
					enabledNamespaces: &[]string{"ns2"},
				},
			},
			remoteConfigIDsToDisable: []string{"abc"},
			clusterName:              "dev",
			expectedConfig: ssiConfig{
				enabled:           true,
				enabledNamespaces: []string{"ns2"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled := tt.localConfig.enabled
			enabledNamespaces := tt.localConfig.enabledNamespaces
			disabledNamespaces := tt.localConfig.disabledNamespaces
			c := newInstrumentationConfigurationCache(&enabled, &enabledNamespaces, &disabledNamespaces, "")

			c.orderedRevisions = tt.orderedRevisions
			c.enabledRevisions = tt.enabledRevisions

			for _, configId := range tt.remoteConfigIDsToDisable {
				c.delete(configId)
			}
			require.Equal(t, tt.expectedConfig.enabled, c.currentConfiguration.enabled)
			require.Equal(t, tt.expectedConfig.enabledNamespaces, c.currentConfiguration.enabledNamespaces)
			require.Equal(t, tt.expectedConfig.disabledNamespaces, c.currentConfiguration.disabledNamespaces)
		})
	}
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		name           string
		localConfig    ssiConfig
		remoteRequests []Request
		clusterName    string
		expectedConfig ssiConfig
	}{
		{
			name: "Single rule: enable instrumentation in the cluster, local instrumentation is off",
			localConfig: ssiConfig{
				enabled: false,
			},
			remoteRequests: []Request{
				{
					ID:          "abc",
					Revision:    123,
					RcVersion:   1,
					Action:      "enable",
					LibConfig:   common.LibConfig{},
					K8sTargetV2: &K8sTargetV2{ClusterTargets: []K8sClusterTarget{{ClusterName: "dev", Enabled: newTrue()}}},
				},
			},
			clusterName: "dev",
			expectedConfig: ssiConfig{
				enabled: true,
			},
		},
		{
			name: "Single rule: enable instrumentation in the namespace, local instrumentation is off",
			localConfig: ssiConfig{
				enabled: false,
			},
			remoteRequests: []Request{
				{
					ID:          "abc",
					Revision:    123,
					RcVersion:   1,
					Action:      "enable",
					LibConfig:   common.LibConfig{},
					K8sTargetV2: &K8sTargetV2{ClusterTargets: []K8sClusterTarget{{ClusterName: "dev", Enabled: newTrue(), EnabledNamespaces: &[]string{"ns1"}}}},
				},
			},
			clusterName: "dev",
			expectedConfig: ssiConfig{
				enabled:           true,
				enabledNamespaces: []string{"ns1"},
			},
		},
		{
			name: "Multiple rules: enable instrumentation in the namespaces",
			localConfig: ssiConfig{
				enabled: false,
			},
			remoteRequests: []Request{
				{
					ID:          "abc",
					Revision:    123,
					RcVersion:   1,
					Action:      "enable",
					LibConfig:   common.LibConfig{},
					K8sTargetV2: &K8sTargetV2{ClusterTargets: []K8sClusterTarget{{ClusterName: "dev", Enabled: newTrue(), EnabledNamespaces: &[]string{"ns1"}}}},
				},
				{
					ID:          "def",
					Revision:    125,
					RcVersion:   1,
					Action:      "enable",
					LibConfig:   common.LibConfig{},
					K8sTargetV2: &K8sTargetV2{ClusterTargets: []K8sClusterTarget{{ClusterName: "dev", Enabled: newTrue(), EnabledNamespaces: &[]string{"ns2"}}}},
				},
			},
			clusterName: "dev",
			expectedConfig: ssiConfig{
				enabled:           true,
				enabledNamespaces: []string{"ns1", "ns2"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled := tt.localConfig.enabled
			enabledNamespaces := tt.localConfig.enabledNamespaces
			disabledNamespaces := tt.localConfig.disabledNamespaces
			c := newInstrumentationConfigurationCache(&enabled, &enabledNamespaces, &disabledNamespaces, tt.clusterName)

			for _, req := range tt.remoteRequests {
				c.update(req)
			}
			require.Equal(t, tt.expectedConfig.enabled, c.currentConfiguration.enabled)
			require.Equal(t, tt.expectedConfig.enabledNamespaces, c.currentConfiguration.enabledNamespaces)
			require.Equal(t, tt.expectedConfig.disabledNamespaces, c.currentConfiguration.disabledNamespaces)
		})
	}
}

func newTrue() *bool {
	b := true
	return &b
}

func newFalse() *bool {
	b := false
	return &b
}

func newString(s string) *string {
	return &s
}
