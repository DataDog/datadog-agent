// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

func podSpec(sc *corev1.SecurityContext) *corev1.PodSpec {
	return &corev1.PodSpec{Containers: []corev1.Container{{Name: "app", SecurityContext: sc}}}
}

func TestRenderCapabilitiesKeepsTheManifestAdds(t *testing.T) {
	spec := podSpec(&corev1.SecurityContext{Capabilities: &corev1.Capabilities{
		Add:  []corev1.Capability{"NET_BIND_SERVICE", "CAP_SYS_TIME"},
		Drop: []corev1.Capability{"SYS_TIME"},
	}})
	sc, reason := Render(ControlCapabilities, "app", spec)
	assert.Empty(t, reason)
	assert.Equal(t, &corev1.Capabilities{Add: []corev1.Capability{"NET_BIND_SERVICE"}, Drop: []corev1.Capability{"ALL"}}, sc.Capabilities,
		"a capability the manifest also drops was never held")
	assert.Equal(t, []corev1.Capability{"SYS_TIME"}, spec.Containers[0].SecurityContext.Capabilities.Drop, "spec is not modified")

	sc, _ = Render(ControlCapabilities, "app", podSpec(nil))
	assert.Equal(t, &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}, sc.Capabilities)
}

func TestRenderRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		control Control
		sc      *corev1.SecurityContext
	}{
		"privileged": {ControlNoPrivilegeEscalation, &corev1.SecurityContext{Privileged: ptr.To(true)}},
		"adds ALL":   {ControlCapabilities, &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"ALL"}}}},
		"uid 0":      {ControlNonRoot, &corev1.SecurityContext{RunAsUser: ptr.To(int64(0))}},
	} {
		_, reason := Render(tc.control, "app", podSpec(tc.sc))
		assert.NotEmpty(t, reason, name)
	}
	_, reason := Render(ControlNonRoot, "missing", podSpec(nil))
	assert.Equal(t, "container not found", reason)
}

func TestRenderH2Controls(t *testing.T) {
	sc, _ := Render(ControlNonRoot, "app", podSpec(nil))
	assert.Equal(t, &corev1.SecurityContext{RunAsNonRoot: ptr.To(true)}, sc)
	sc, _ = Render(ControlNoPrivilegeEscalation, "app", podSpec(nil))
	assert.Equal(t, &corev1.SecurityContext{AllowPrivilegeEscalation: ptr.To(false)}, sc)
}

func TestSetsControl(t *testing.T) {
	assert.True(t, SetsControl(ControlCapabilities, "app", podSpec(&corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"all"}}})))
	assert.False(t, SetsControl(ControlCapabilities, "app", podSpec(nil)))
	spec := podSpec(nil)
	spec.SecurityContext = &corev1.PodSecurityContext{RunAsNonRoot: ptr.To(true)}
	assert.True(t, SetsControl(ControlNonRoot, "app", spec), "the pod's securityContext is the default")
	assert.True(t, SetsControl(ControlNoPrivilegeEscalation, "app", podSpec(&corev1.SecurityContext{AllowPrivilegeEscalation: ptr.To(false)})))
}
