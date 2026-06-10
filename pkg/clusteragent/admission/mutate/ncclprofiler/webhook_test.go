// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package ncclprofiler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestWebhook_DisabledByDefault(t *testing.T) {
	cfg := configmock.New(t)

	w := NewWebhook(cfg)

	assert.False(t, w.IsEnabled(), "webhook must be disabled when no admission_controller.nccl_profiler.enabled is set")
}

func TestWebhook_EnabledButNoInjectorImage_DisablesItself(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("admission_controller.nccl_profiler.enabled", true)
	// admission_controller.nccl_profiler.injector_image deliberately left empty

	w := NewWebhook(cfg)

	assert.False(t, w.IsEnabled(),
		"webhook must self-disable when enabled=true but injector_image is empty (no broken image refs in pods)")
}

func TestWebhook_EnabledWithInjectorImage(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("admission_controller.nccl_profiler.enabled", true)
	cfg.SetInTest("admission_controller.nccl_profiler.injector_image",
		"376334461865.dkr.ecr.us-east-1.amazonaws.com/nccl-profiler-injector:latest")

	w := NewWebhook(cfg)

	assert.True(t, w.IsEnabled())
	assert.Equal(t, "nccl_profiler", w.Name())
	assert.Equal(t, "/inject-nccl-profiler", w.Endpoint())
}

func TestWebhook_LabelSelectorTargetsOnlyOptedInPods(t *testing.T) {
	cfg := configmock.New(t)
	w := NewWebhook(cfg)

	nsSel, objSel := w.LabelSelectors(false)

	require.NotNil(t, nsSel, "namespace selector must exclude system namespaces")
	require.Len(t, nsSel.MatchExpressions, 1)
	assert.Equal(t, common.NamespaceLabelKey, nsSel.MatchExpressions[0].Key)
	assert.Equal(t, metav1.LabelSelectorOpNotIn, nsSel.MatchExpressions[0].Operator)
	assert.ElementsMatch(t, mutatecommon.DefaultDisabledNamespaces(), nsSel.MatchExpressions[0].Values,
		"namespace selector must exclude exactly DefaultDisabledNamespaces (kube-system + agent ns)")

	require.NotNil(t, objSel)
	assert.Equal(t, "true", objSel.MatchLabels[EnabledLabel],
		"object selector must require admission.datadoghq.com/nccl-profiler.enabled=true")
}

// TestWebhook_LabelSelector_FallbackFailsClosed covers the K8s 1.10-1.14
// fallback path. When useNamespaceSelector=true, the API server ignores
// objectSelector and pod-level opt-in cannot be enforced via namespaceSelector.
// The webhook must fail closed: return a namespaceSelector that matches no
// namespace so no pod gets mutated.
func TestWebhook_LabelSelector_FallbackFailsClosed(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("admission_controller.nccl_profiler.enabled", true)
	cfg.SetInTest("admission_controller.nccl_profiler.injector_image", "registry.example/img:tag")

	nsSel, objSel := NewWebhook(cfg).LabelSelectors(true)

	require.Nil(t, objSel, "fallback mode: objectSelector must not be returned (K8s 1.10-1.14 ignores it)")
	require.NotNil(t, nsSel)
	require.Len(t, nsSel.MatchExpressions, 1)
	expr := nsSel.MatchExpressions[0]
	assert.Equal(t, common.NamespaceLabelKey, expr.Key)
	assert.Equal(t, metav1.LabelSelectorOpIn, expr.Operator,
		"fallback selector must require an impossible namespace label so it never matches")
	assert.NotContains(t, expr.Values, "default",
		"fallback selector must not match any real namespace")
}

func TestValidSocketConfig(t *testing.T) {
	for _, tc := range []struct {
		name       string
		hostDir    string
		clientDir  string
		socketPath string
		want       bool
	}{
		{"defaults", "/var/run/datadog", "/var/run/datadog", "/var/run/datadog/nccl.socket", true},
		{"decoupled dirs", "/var/run/datadog-agent", "/var/run/datadog", "/var/run/datadog/nccl.socket", true},
		{"empty hostDir", "", "/var/run/datadog", "/var/run/datadog/nccl.socket", false},
		{"empty clientDir", "/var/run/datadog", "", "/var/run/datadog/nccl.socket", false},
		{"empty socket_path", "/var/run/datadog", "/var/run/datadog", "", false},
		{"relative hostDir", "var/run/datadog", "/var/run/datadog", "/var/run/datadog/nccl.socket", false},
		{"relative clientDir", "/var/run/datadog", "var/run/datadog", "/var/run/datadog/nccl.socket", false},
		{"hostDir is /", "/", "/var/run/datadog", "/var/run/datadog/nccl.socket", false},
		{"clientDir is /", "/var/run/datadog", "/", "/var/run/datadog/nccl.socket", false},
		{"socket_path relative (basename-only)", "/var/run/datadog", "/var/run/datadog", "nccl.socket", false},
		{"socket_path trailing slash", "/var/run/datadog", "/var/run/datadog", "/var/run/datadog/", false},
		{"socket_path is /", "/var/run/datadog", "/var/run/datadog", "/", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, validSocketConfig(tc.hostDir, tc.clientDir, tc.socketPath))
		})
	}
}

func TestWebhook_InvalidSocketConfigDisablesItself(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  string
		val  string
	}{
		{"relative socket_path", "gpu.nccl.socket_path", "nccl.socket"},
		{"trailing-slash socket_path", "gpu.nccl.socket_path", "/var/run/datadog/"},
		{"root client socket_dir", "admission_controller.nccl_profiler.socket_dir", "/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("admission_controller.nccl_profiler.enabled", true)
			cfg.SetInTest("admission_controller.nccl_profiler.injector_image", "registry.example/img:tag")
			cfg.SetInTest(tc.key, tc.val)
			assert.False(t, NewWebhook(cfg).IsEnabled())
		})
	}
}

// TestWebhook_LabelSelector_MutateUnlabelled covers blanket mode: when the
// per-webhook (or global) mutate_unlabelled knob is set, the objectSelector
// flips from "label=true required" to "label!=false". cwsinstrumentation has
// the same shape.
func TestWebhook_LabelSelector_MutateUnlabelled(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  string
	}{
		{name: "per-webhook knob", key: "admission_controller.nccl_profiler.mutate_unlabelled"},
		{name: "global knob", key: "admission_controller.mutate_unlabelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest(tc.key, true)

			_, objSel := NewWebhook(cfg).LabelSelectors(false)

			require.NotNil(t, objSel)
			assert.Empty(t, objSel.MatchLabels, "blanket mode must not require label=true")
			require.Len(t, objSel.MatchExpressions, 1)
			expr := objSel.MatchExpressions[0]
			assert.Equal(t, EnabledLabel, expr.Key)
			assert.Equal(t, metav1.LabelSelectorOpNotIn, expr.Operator)
			assert.Equal(t, []string{"false"}, expr.Values)
		})
	}
}
