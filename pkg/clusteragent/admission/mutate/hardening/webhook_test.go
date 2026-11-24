// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/hardening"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const cluster = "cluster-1"

func policy(id, control, state, listed string) []byte {
	return []byte(fmt.Sprintf(`{"type":"policy","id":%q,"control":%q,"state":%q,
		"clusters":[{"id":%q,"added_at":"2026-11-01T00:00:00Z"}],
		"limits":{"first_batch":1,"hourly_cap":1,"pause_threshold":1}}`, id, control, state, listed))
}

func newWebhook(t *testing.T, configs ...[]byte) *Webhook {
	store := hardening.NewStore(cluster)
	updates := map[string]state.RawConfig{}
	for i, c := range configs {
		updates[fmt.Sprintf("path/%d", i)] = state.RawConfig{Config: c}
	}
	store.OnRCUpdate(updates, func(_ string, st state.ApplyStatus) { require.Empty(t, st.Error) })
	cfg := configmock.New(t)
	cfg.SetInTest("admission_controller.hardening.enabled", true)
	return NewWebhook(cfg, store)
}

// pod returns a pod with an "app" container whose template has marks.
func pod(marks string, sc *corev1.SecurityContext) *corev1.Pod {
	return mutatecommon.FakePodSpec{
		NS:          "shop",
		ParentKind:  "replicaset",
		ParentName:  "web-5d4f8b7c9",
		Labels:      map[string]string{hardening.EnabledLabel: "true"},
		Annotations: map[string]string{hardening.PoliciesAnnotation: marks},
		Containers:  []corev1.Container{{Name: "app", SecurityContext: sc}},
	}.Create()
}

func applied(t *testing.T, p *corev1.Pod) map[string]string {
	var m map[string]string
	require.NoError(t, json.Unmarshal([]byte(p.Annotations[hardening.AppliedAnnotation]), &m))
	return m
}

func TestWebhookAppliesEveryMarkedPolicy(t *testing.T) {
	w := newWebhook(t, policy("hp-caps", "capabilities", "enforce", cluster), policy("hp-root", "non_root", "enforce", cluster))
	p := pod(`{"hp-caps":["app"],"hp-root":["app"]}`, &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"NET_BIND_SERVICE"}}})

	changed, err := w.mutate(p, "shop", nil)
	require.NoError(t, err)
	assert.True(t, changed)
	sc := hardening.FindContainer(&p.Spec, "app").SecurityContext
	assert.Equal(t, &corev1.Capabilities{Add: []corev1.Capability{"NET_BIND_SERVICE"}, Drop: []corev1.Capability{"ALL"}}, sc.Capabilities)
	assert.Equal(t, ptr.To(true), sc.RunAsNonRoot)
	assert.Nil(t, hardening.FindContainer(&p.Spec, "pod").SecurityContext, "unmarked containers are untouched")
	assert.Equal(t, map[string]string{"hp-caps/app": "applied", "hp-root/app": "applied"}, applied(t, p))
}

func TestWebhookSkips(t *testing.T) {
	w := newWebhook(t, policy("hp-stopped", "capabilities", "stopped", cluster), policy("hp-other", "non_root", "enforce", "other-cluster"))
	p := pod(`{"hp-stopped":["app"],"hp-other":["app"],"hp-unknown":["app"],"hp-gone":["missing"]}`, nil)

	_, err := w.mutate(p, "shop", nil)
	require.NoError(t, err)
	assert.Nil(t, hardening.FindContainer(&p.Spec, "app").SecurityContext)
	results := applied(t, p)
	assert.Equal(t, hardening.AppliedUnknown, results["hp-unknown/app"])
	assert.Equal(t, hardening.AppliedUnknown, results["hp-gone/missing"])
	assert.Contains(t, results["hp-stopped/app"], "skipped")
	assert.Contains(t, results["hp-other/app"], "skipped")
}

func TestWebhookIgnoresUnmarkedPods(t *testing.T) {
	w := newWebhook(t, policy("hp-caps", "capabilities", "enforce", cluster))
	for _, marks := range []string{"", `{"hp-caps":[]}`, "not json"} {
		p := pod(marks, nil)
		changed, err := w.mutate(p, "shop", nil)
		require.NoError(t, err)
		assert.False(t, changed, marks)
		assert.NotContains(t, p.Annotations, hardening.AppliedAnnotation)
	}
}
