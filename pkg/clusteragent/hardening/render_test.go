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

func specWith(sc *corev1.SecurityContext) *corev1.PodSpec {
	return &corev1.PodSpec{Containers: []corev1.Container{{Name: "other"}, {Name: "app", SecurityContext: sc}}}
}

func capsRequest(add ...string) *Request {
	return &Request{Control: ControlCapabilities, Target: Target{Container: "app"}, Parameters: Parameters{CapabilitiesAdd: add}}
}

func TestRenderCapabilities(t *testing.T) {
	tests := []struct {
		name     string
		declared *corev1.Capabilities
		add      []string
		wantAdd  []corev1.Capability
		wantSkip bool
	}{
		{name: "runtime default capability", add: []string{"NET_BIND_SERVICE"}, wantAdd: []corev1.Capability{"NET_BIND_SERVICE"}},
		{name: "drop everything", add: nil, wantAdd: nil},
		{name: "sorted and deduplicated", add: []string{"SETUID", "CHOWN", "SETUID"}, wantAdd: []corev1.Capability{"CHOWN", "SETUID"}},
		{name: "not granted today", add: []string{"SYS_ADMIN"}, wantSkip: true},
		{name: "declared add", declared: &corev1.Capabilities{Add: []corev1.Capability{"CAP_SYS_ADMIN"}}, add: []string{"SYS_ADMIN"}, wantAdd: []corev1.Capability{"SYS_ADMIN"}},
		{name: "declared add ALL", declared: &corev1.Capabilities{Add: []corev1.Capability{"ALL"}}, add: []string{"SYS_ADMIN"}, wantAdd: []corev1.Capability{"SYS_ADMIN"}},
		{name: "declared drop ALL", declared: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}, Add: []corev1.Capability{"CHOWN"}}, add: []string{"NET_BIND_SERVICE"}, wantSkip: true},
		{name: "declared drop ALL keeps added", declared: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}, Add: []corev1.Capability{"CHOWN"}}, add: []string{"CHOWN"}, wantAdd: []corev1.Capability{"CHOWN"}},
		{name: "declared drop wins over add", declared: &corev1.Capabilities{Add: []corev1.Capability{"NET_RAW"}, Drop: []corev1.Capability{"NET_RAW"}}, add: []string{"NET_RAW"}, wantSkip: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := specWith(&corev1.SecurityContext{Capabilities: tt.declared, RunAsUser: ptr.To(int64(1000))})
			sc, reason := Render(capsRequest(tt.add...), spec)
			if tt.wantSkip {
				assert.Nil(t, sc)
				assert.NotEmpty(t, reason)
				return
			}
			assert.Empty(t, reason)
			assert.Equal(t, &corev1.Capabilities{Add: tt.wantAdd, Drop: []corev1.Capability{"ALL"}}, sc.Capabilities)
			assert.Equal(t, ptr.To(int64(1000)), sc.RunAsUser, "other fields are preserved")
		})
	}
}

func TestRenderCommonSkips(t *testing.T) {
	_, reason := Render(capsRequest(), specWith(&corev1.SecurityContext{Privileged: ptr.To(true)}))
	assert.Equal(t, "container is privileged", reason)

	req := capsRequest()
	req.Target.Container = "missing"
	_, reason = Render(req, specWith(nil))
	assert.Equal(t, "container not found", reason)

	// Init containers are never targeted.
	spec := &corev1.PodSpec{InitContainers: []corev1.Container{{Name: "app"}}}
	_, reason = Render(capsRequest(), spec)
	assert.Equal(t, "container not found", reason)
}

func TestRenderReadOnlyRootFS(t *testing.T) {
	req := &Request{Control: ControlReadOnlyRootFS, Target: Target{Container: "app"}}
	declared := &corev1.SecurityContext{ReadOnlyRootFilesystem: ptr.To(false)}
	spec := specWith(declared)

	sc, reason := Render(req, spec)
	assert.Empty(t, reason)
	assert.Equal(t, ptr.To(true), sc.ReadOnlyRootFilesystem, "an explicit false is overridden")
	assert.Equal(t, ptr.To(false), declared.ReadOnlyRootFilesystem, "the input is not modified")
}

func TestRenderSeccomp(t *testing.T) {
	req := &Request{Control: ControlSeccompRuntimeDefault, Target: Target{Container: "app"}}
	runtimeDefault := &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}
	localhost := &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeLocalhost, LocalhostProfile: ptr.To("p.json")}

	sc, reason := Render(req, specWith(nil))
	assert.Empty(t, reason)
	assert.Equal(t, runtimeDefault, sc.SeccompProfile)

	sc, reason = Render(req, specWith(&corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeUnconfined}}))
	assert.Empty(t, reason)
	assert.Equal(t, runtimeDefault, sc.SeccompProfile)

	_, reason = Render(req, specWith(&corev1.SecurityContext{SeccompProfile: localhost}))
	assert.NotEmpty(t, reason)

	spec := specWith(nil)
	spec.SecurityContext = &corev1.PodSecurityContext{SeccompProfile: localhost}
	_, reason = Render(req, spec)
	assert.NotEmpty(t, reason, "a pod-level Localhost profile is not replaced either")
}
