// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestGetBundleInheritedAllowedActions(t *testing.T) {
	tests := []struct {
		name                     string
		actionsAllowlist         map[string]sets.Set[string]
		expectedInheritedActions map[string]sets.Set[string]
	}{
		{
			name: "returns special actions for existing bundle",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string]("action1"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string]("testConnection", "enrichScript"),
			},
		},
		{
			name: "returns special actions for sibling bundles",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.kubernetes.apps": sets.New[string]("action3"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{
				"com.datadoghq.kubernetes.core": sets.New[string]("testConnection"),
			},
		},
		{
			name: "returns empty when bundle not in allowlist",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.other.bundle": sets.New[string]("action1"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{},
		},
		{
			name: "returns empty when bundle has empty set",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.script": sets.New[string](),
			},
			expectedInheritedActions: map[string]sets.Set[string]{},
		},
		{
			name:                     "returns empty for empty allowlist",
			actionsAllowlist:         map[string]sets.Set[string]{},
			expectedInheritedActions: map[string]sets.Set[string]{},
		},
		{
			name: "returns special actions for multiple matching bundles",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.other.bundle":              sets.New[string]("otherAction"),
				"com.datadoghq.script":          sets.New[string]("action1"),
				"com.datadoghq.gitlab.users":    sets.New[string]("action2"),
				"com.datadoghq.kubernetes.core": sets.New[string]("action3"),
				"com.datadoghq.kubernetes.apps": sets.New[string]("action4"),
				"com.datadoghq.remoteaction":    sets.New[string]("action5"),
				"com.datadoghq.mongodb":         sets.New[string]("action6"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{
				"com.datadoghq.script":                sets.New[string]("testConnection", "enrichScript"),
				"com.datadoghq.gitlab.users":          sets.New[string]("testConnection"),
				"com.datadoghq.kubernetes.core":       sets.New[string]("testConnection"),
				"com.datadoghq.mongodb":               sets.New[string]("testConnection"),
				"com.datadoghq.remoteaction":          sets.New[string]("testConnection"),
				"com.datadoghq.remoteaction.internal": sets.New[string]("prepareEncryption"),
			},
		},
		{
			name: "returns empty for similar looking bundle	",
			actionsAllowlist: map[string]sets.Set[string]{
				"com.datadoghq.dd":           sets.New[string]("action1"),
				"com.datadoghq.dd.subbundle": sets.New[string]("action2"),
			},
			expectedInheritedActions: map[string]sets.Set[string]{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetBundleInheritedAllowedActions(tt.actionsAllowlist)
			assert.Equal(t, tt.expectedInheritedActions, result)
		})
	}
}

func TestIsActionAllowed(t *testing.T) {
	tests := []struct {
		name      string
		allowlist []string
		bundleID  string
		action    string
		allowed   bool
	}{
		{"exact action", []string{"com.datadoghq.kubernetes.core.getPod"}, "com.datadoghq.kubernetes.core", "getPod", true},
		{"different action", []string{"com.datadoghq.kubernetes.core.getPod"}, "com.datadoghq.kubernetes.core", "deletePod", false},
		{"bundle wildcard", []string{"com.datadoghq.kubernetes.core.*"}, "com.datadoghq.kubernetes.core", "deletePod", true},
		{"kubernetes wildcard", []string{"com.datadoghq.kubernetes.*"}, "com.datadoghq.kubernetes.core", "deletePod", true},
		{"gitlab wildcard", []string{"com.datadoghq.gitlab.*"}, "com.datadoghq.gitlab.projects", "listProjects", true},
		{"root bundle wildcard", []string{"com.datadoghq.http.*"}, "com.datadoghq.http", "request", true},
		{"nested bundle wildcard", []string{"com.datadoghq.authoredscripts.*"}, "com.datadoghq.authoredscripts.helm.repo", "add", true},
		{"parent wildcard with exact bundle entry", []string{"com.datadoghq.kubernetes.*", "com.datadoghq.kubernetes.core.getPod"}, "com.datadoghq.kubernetes.core", "deletePod", true},
		{"other integration", []string{"com.datadoghq.kubernetes.*"}, "com.datadoghq.gitlab.projects", "listProjects", false},
		{"similar integration prefix", []string{"com.datadoghq.kubernetes.*"}, "com.datadoghq.kubernetesother.core", "getPod", false},
		{"sibling bundle", []string{"com.datadoghq.kubernetes.core.*"}, "com.datadoghq.kubernetes.apps", "listDeployment", false},
		{"exact action does not cover children", []string{"com.datadoghq.kubernetes.getPod"}, "com.datadoghq.kubernetes.core", "getPod", false},
		{"global wildcard", []string{"*"}, "com.datadoghq.kubernetes.core", "getPod", false},
		{"namespace wildcard", []string{"com.datadoghq.*"}, "com.datadoghq.kubernetes.core", "getPod", false},
		{"namespace wildcard on namespace", []string{"com.datadoghq.*"}, "com.datadoghq", "getPod", false},
		{"empty integration", []string{"com.datadoghq..*"}, "com.datadoghq..core", "getPod", false},
		{"other namespace", []string{"com.other.kubernetes.*"}, "com.other.kubernetes.core", "getPod", false},
		{"embedded wildcard", []string{"com.datadoghq.kube*.*"}, "com.datadoghq.kubernetes.core", "getPod", false},
		{"empty allowlist", nil, "com.datadoghq.kubernetes.core", "getPod", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetInTest(par.DefaultActionsEnabled, false)
			mockConfig.SetInTest(par.ActionsAllowlist, tt.allowlist)
			allowlist := MakeActionsAllowlist(mockConfig)
			assert.Equal(t, tt.allowed, allowlist.IsActionAllowed(tt.bundleID, tt.action))
		})
	}
}

func TestMakeActionsAllowlistDefaultActionsEnabled(t *testing.T) {
	t.Run("cluster agent default actions are included when default_actions_enabled is true", func(t *testing.T) {
		flavor.SetFlavor(flavor.ClusterAgent)
		defer flavor.SetFlavor(flavor.DefaultAgent)

		mockConfig := configmock.New(t)
		mockConfig.SetInTest(par.ActionsAllowlist, []string{})
		mockConfig.SetInTest(par.DefaultActionsEnabled, true)

		allowlist := MakeActionsAllowlist(mockConfig)

		assert.True(t, allowlist["com.datadoghq.kubernetes.apps"].Has("listDeployment"))
		assert.True(t, allowlist["com.datadoghq.kubernetes.core"].Has("getPod"))
		assert.True(t, allowlist["com.datadoghq.kubernetes.core"].Has("getPodLogs"))
		assert.True(t, allowlist["com.datadoghq.kubernetes.batch"].Has("getJob"))
		// common actions should also be present
		assert.True(t, allowlist["com.datadoghq.remoteaction.networks"].Has("runNetworkPath"))
		assert.True(t, allowlist["com.datadoghq.remoteaction.rshell"].Has("runCommand"))
		// agent read-only diagnostics are node-agent only and must NOT be present
		_, hasAgentBundle := allowlist["com.datadoghq.remoteaction.datadogagent"]
		assert.False(t, hasAgentBundle)
		// inherited actions should also be present for the kubernetes prefix
		assert.True(t, allowlist["com.datadoghq.kubernetes.core"].Has("testConnection"))
	})

	t.Run("non-cluster-agent flavor returns common default actions only", func(t *testing.T) {
		flavor.SetFlavor(flavor.DefaultAgent)

		mockConfig := configmock.New(t)
		mockConfig.SetInTest(par.ActionsAllowlist, []string{})
		mockConfig.SetInTest(par.DefaultActionsEnabled, true)

		allowlist := MakeActionsAllowlist(mockConfig)

		// common actions should be present
		assert.True(t, allowlist["com.datadoghq.remoteaction.networks"].Has("runNetworkPath"))
		assert.True(t, allowlist["com.datadoghq.remoteaction.rshell"].Has("runCommand"))
		// agent read-only diagnostics should be present
		assert.True(t, allowlist["com.datadoghq.remoteaction.datadogagent"].Has("getStatus"))
		assert.True(t, allowlist["com.datadoghq.remoteaction.datadogagent"].Has("getConfig"))
		assert.True(t, allowlist["com.datadoghq.remoteaction.datadogagent"].Has("getDiagnose"))
		// cluster-agent-specific actions should NOT be present
		_, hasK8sApps := allowlist["com.datadoghq.kubernetes.apps"]
		assert.False(t, hasK8sApps)
	})

	t.Run("default actions are excluded when default_actions_enabled is false", func(t *testing.T) {
		flavor.SetFlavor(flavor.ClusterAgent)
		defer flavor.SetFlavor(flavor.DefaultAgent)

		mockConfig := configmock.New(t)
		mockConfig.SetInTest(par.ActionsAllowlist, []string{})
		mockConfig.SetInTest(par.DefaultActionsEnabled, false)

		allowlist := MakeActionsAllowlist(mockConfig)

		assert.Empty(t, allowlist)
	})

	t.Run("cluster agent default actions merge with explicit allowlist", func(t *testing.T) {
		flavor.SetFlavor(flavor.ClusterAgent)
		defer flavor.SetFlavor(flavor.DefaultAgent)

		mockConfig := configmock.New(t)
		mockConfig.SetInTest(par.ActionsAllowlist, []string{"com.datadoghq.http.sendRequest"})
		mockConfig.SetInTest(par.DefaultActionsEnabled, true)

		allowlist := MakeActionsAllowlist(mockConfig)

		assert.True(t, allowlist["com.datadoghq.kubernetes.apps"].Has("listDeployment"))
		assert.True(t, allowlist["com.datadoghq.http"].Has("sendRequest"))
	})

	t.Run("explicit allowlist works without default actions", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetInTest(par.ActionsAllowlist, []string{"com.datadoghq.http.sendRequest"})
		mockConfig.SetInTest(par.DefaultActionsEnabled, false)

		allowlist := MakeActionsAllowlist(mockConfig)

		assert.True(t, allowlist["com.datadoghq.http"].Has("sendRequest"))
		_, hasK8sApps := allowlist["com.datadoghq.kubernetes.apps"]
		assert.False(t, hasK8sApps)
	})

	t.Run("kubeactions bundle is auto-allowed when kubeactions.enabled is true", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetInTest(par.ActionsAllowlist, []string{})
		mockConfig.SetInTest(par.DefaultActionsEnabled, false)
		mockConfig.SetInTest("kubeactions.enabled", true)

		allowlist := MakeActionsAllowlist(mockConfig)

		for _, action := range []string{"deletePod", "restartDeployment", "patchDeployment", "rollbackDeployment", "getResource"} {
			assert.True(t, allowlist["com.datadoghq.kubernetes.kubeactions"].Has(action), "expected %s to be allowed", action)
		}
	})

	t.Run("kubeactions bundle is not allowed when kubeactions.enabled is false", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetInTest(par.ActionsAllowlist, []string{})
		mockConfig.SetInTest(par.DefaultActionsEnabled, false)
		mockConfig.SetInTest("kubeactions.enabled", false)

		allowlist := MakeActionsAllowlist(mockConfig)

		_, has := allowlist["com.datadoghq.kubernetes.kubeactions"]
		assert.False(t, has)
	})

	t.Run("kubeactions bundle is not allowed when its private action runner path is disabled", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetInTest(par.ActionsAllowlist, []string{})
		mockConfig.SetInTest(par.DefaultActionsEnabled, false)
		mockConfig.SetInTest("kubeactions.enabled", true)
		mockConfig.SetInTest("kubeactions.private_action_runner.enabled", false)

		allowlist := MakeActionsAllowlist(mockConfig)

		_, has := allowlist["com.datadoghq.kubernetes.kubeactions"]
		assert.False(t, has)
	})

	t.Run("explicit allowlist entries still apply when the kubeactions private action runner path is disabled", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetInTest(par.ActionsAllowlist, []string{"com.datadoghq.kubernetes.kubeactions.getResource"})
		mockConfig.SetInTest(par.DefaultActionsEnabled, false)
		mockConfig.SetInTest("kubeactions.enabled", true)
		mockConfig.SetInTest("kubeactions.private_action_runner.enabled", false)

		allowlist := MakeActionsAllowlist(mockConfig)

		assert.True(t, allowlist["com.datadoghq.kubernetes.kubeactions"].Has("getResource"))
		assert.False(t, allowlist["com.datadoghq.kubernetes.kubeactions"].Has("deletePod"))
	})
}
