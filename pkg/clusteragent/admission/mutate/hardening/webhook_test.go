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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/hardening"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const cluster = "cluster-1"

func request(id, action, control, deployment string, expiresIn time.Duration, params string) []byte {
	return []byte(fmt.Sprintf(`{"id":%q,"action":%q,"control":%q,
		"k8s_target":{"cluster":%q,"kind":"Deployment","namespace":"shop","name":%q,"uid":"uid-1","container":"app"},
		"parameters":%s,"expires_at":%q}`,
		id, action, control, cluster, deployment, params, time.Now().Add(expiresIn).Format(time.RFC3339)))
}

func newWebhook(t *testing.T, reqs ...[]byte) *Webhook {
	store := hardening.NewStore(cluster)
	updates := map[string]state.RawConfig{}
	for i, r := range reqs {
		updates[fmt.Sprintf("path/%d", i)] = state.RawConfig{Config: r}
	}
	store.OnRCUpdate(updates, func(_ string, st state.ApplyStatus) { require.Empty(t, st.Error) })
	cfg := configmock.New(t)
	cfg.SetInTest("admission_controller.hardening.enabled", true)
	return NewWebhook(cfg, store)
}

// pod returns a pod created by a ReplicaSet of Deployment "web" that lists ids.
func pod(ids string, sc *corev1.SecurityContext) *corev1.Pod {
	return mutatecommon.FakePodSpec{
		NS:          "shop",
		ParentKind:  "replicaset",
		ParentName:  "web-5d4f8b7c9",
		Labels:      map[string]string{hardening.EnabledLabel: "true"},
		Annotations: map[string]string{hardening.RequestsAnnotation: ids},
		Containers:  []corev1.Container{{Name: "app", SecurityContext: sc}},
	}.Create()
}

func applied(t *testing.T, p *corev1.Pod) map[string]string {
	var m map[string]string
	require.NoError(t, json.Unmarshal([]byte(p.Annotations[hardening.AppliedAnnotation]), &m))
	return m
}

func TestWebhookAppliesCapabilities(t *testing.T) {
	w := newWebhook(t, request("req-1", "trial", "capabilities", "web", time.Hour, `{"capabilities_add":["NET_BIND_SERVICE"]}`))
	p := pod("req-1", nil)

	changed, err := w.mutate(p, "shop", nil)
	require.NoError(t, err)
	assert.True(t, changed)
	app := hardening.FindContainer(&p.Spec, "app")
	assert.Equal(t, &corev1.Capabilities{Add: []corev1.Capability{"NET_BIND_SERVICE"}, Drop: []corev1.Capability{"ALL"}}, app.SecurityContext.Capabilities)
	assert.Nil(t, hardening.FindContainer(&p.Spec, "pod").SecurityContext, "other containers are untouched")
	assert.Equal(t, map[string]string{"req-1": "applied"}, applied(t, p))
}

func TestWebhookForgedAnnotation(t *testing.T) {
	w := newWebhook(t, request("req-1", "trial", "read_only_root_fs", "web", time.Hour, `{}`))
	p := pod("req-1", nil)
	p.OwnerReferences[0].Name = "api-7c9d8f6b5" // a ReplicaSet of another Deployment

	_, err := w.mutate(p, "shop", nil)
	require.NoError(t, err)
	assert.Nil(t, hardening.FindContainer(&p.Spec, "app").SecurityContext)
	assert.Equal(t, "skipped: request targets another workload", applied(t, p)["req-1"])

	// Same Deployment name in another namespace.
	p = pod("req-1", nil)
	_, err = w.mutate(p, "other", nil)
	require.NoError(t, err)
	assert.Nil(t, hardening.FindContainer(&p.Spec, "app").SecurityContext)
}

func TestWebhookInactiveRequests(t *testing.T) {
	w := newWebhook(t,
		request("expired", "trial", "read_only_root_fs", "web", -time.Minute, `{}`),
		request("reverted", "revert", "read_only_root_fs", "web", time.Hour, `{}`),
	)
	p := pod("expired,reverted,unknown", nil)

	_, err := w.mutate(p, "shop", nil)
	require.NoError(t, err)
	assert.Nil(t, hardening.FindContainer(&p.Spec, "app").SecurityContext)
	for _, id := range []string{"expired", "reverted", "unknown"} {
		assert.Equal(t, "skipped: request is not active", applied(t, p)[id])
	}
}

func TestWebhookSkipsWithRenderReason(t *testing.T) {
	w := newWebhook(t, request("req-1", "trial", "read_only_root_fs", "web", time.Hour, `{}`))
	p := pod("req-1", &corev1.SecurityContext{Privileged: ptr.To(true)})

	_, err := w.mutate(p, "shop", nil)
	require.NoError(t, err)
	assert.Nil(t, hardening.FindContainer(&p.Spec, "app").SecurityContext.ReadOnlyRootFilesystem)
	assert.Equal(t, "skipped: container is privileged", applied(t, p)["req-1"])
}

func TestWebhookNonDeploymentOwner(t *testing.T) {
	w := newWebhook(t, request("req-1", "trial", "read_only_root_fs", "web", time.Hour, `{}`))
	p := mutatecommon.FakePodSpec{
		NS: "shop", ParentKind: "statefulset", ParentName: "web",
		Annotations: map[string]string{hardening.RequestsAnnotation: "req-1"},
		Containers:  []corev1.Container{{Name: "app"}},
	}.Create()

	_, err := w.mutate(p, "shop", nil)
	require.NoError(t, err)
	assert.Nil(t, hardening.FindContainer(&p.Spec, "app").SecurityContext)
}

func TestWebhookTwoRequestsAndReinvocation(t *testing.T) {
	w := newWebhook(t,
		request("caps", "trial", "capabilities", "web", time.Hour, `{"capabilities_add":["CHOWN"]}`),
		request("ro", "trial", "read_only_root_fs", "web", time.Hour, `{}`),
	)
	p := pod("caps,ro", nil)

	_, err := w.mutate(p, "shop", nil)
	require.NoError(t, err)
	sc := hardening.FindContainer(&p.Spec, "app").SecurityContext
	assert.Equal(t, ptr.To(true), sc.ReadOnlyRootFilesystem)
	assert.Equal(t, []corev1.Capability{"CHOWN"}, sc.Capabilities.Add)

	again := p.DeepCopy()
	_, err = w.mutate(again, "shop", nil)
	require.NoError(t, err)
	assert.Equal(t, p, again, "reinvocation changes nothing")
}

func TestWebhookNoRequestsAnnotation(t *testing.T) {
	w := newWebhook(t)
	p := pod("", nil)
	changed, err := w.mutate(p, "shop", nil)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.NotContains(t, p.Annotations, hardening.AppliedAnnotation)
}

func TestWebhookFuncReturnsPatch(t *testing.T) {
	w := newWebhook(t, request("req-1", "trial", "read_only_root_fs", "web", time.Hour, `{}`))
	raw, err := json.Marshal(pod("req-1", nil))
	require.NoError(t, err)

	resp := w.WebhookFunc()(&admission.Request{Object: raw, Namespace: "shop"})
	assert.True(t, resp.Allowed)
	assert.Contains(t, string(resp.Patch), "readOnlyRootFilesystem")
}

func TestWebhookSelectors(t *testing.T) {
	w := newWebhook(t)
	assert.True(t, w.IsEnabled())
	ns, obj := w.LabelSelectors(false)
	assert.Nil(t, ns)
	assert.Equal(t, map[string]string{hardening.EnabledLabel: "true"}, obj.MatchLabels)

	ns, obj = w.LabelSelectors(true)
	assert.NotNil(t, ns, "namespace selector fallback fails closed")
	assert.Nil(t, obj)

	assert.False(t, NewWebhook(configmock.New(t), hardening.NewStore(cluster)).IsEnabled(), "disabled by default")
	cfg := configmock.New(t)
	cfg.SetInTest("admission_controller.hardening.enabled", true)
	assert.False(t, NewWebhook(cfg, nil).IsEnabled(), "disabled without a store")
}
