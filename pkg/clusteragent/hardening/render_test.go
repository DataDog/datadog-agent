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
	return &Request{Control: ControlCapabilities, Workload: Workload{Container: "app"}, Parameters: Parameters{CapabilitiesAdd: add}}
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
		{name: "runtime default unknown", add: []string{"SYS_ADMIN"}, wantSkip: true},
		{name: "outside the intersection", add: []string{"NET_RAW"}, wantSkip: true},
		{name: "declared add", declared: &corev1.Capabilities{Add: []corev1.Capability{"CAP_SYS_ADMIN"}}, add: []string{"SYS_ADMIN"}, wantAdd: []corev1.Capability{"SYS_ADMIN"}},
		{name: "declared add ALL", declared: &corev1.Capabilities{Add: []corev1.Capability{"ALL"}}, add: []string{"SYS_ADMIN"}, wantAdd: []corev1.Capability{"SYS_ADMIN"}},
		{name: "declared drop ALL leaves out the rest", declared: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}, Add: []corev1.Capability{"CHOWN"}}, add: []string{"CHOWN", "NET_BIND_SERVICE"}, wantAdd: []corev1.Capability{"CHOWN"}},
		{name: "declared drop ALL keeps added", declared: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}, Add: []corev1.Capability{"CHOWN"}}, add: []string{"CHOWN"}, wantAdd: []corev1.Capability{"CHOWN"}},
		{name: "declared drop wins over add", declared: &corev1.Capabilities{Add: []corev1.Capability{"NET_RAW"}, Drop: []corev1.Capability{"NET_RAW"}}, add: []string{"NET_RAW"}, wantAdd: nil},
		{name: "add ALL drop ALL grants nothing implicit", declared: &corev1.Capabilities{Add: []corev1.Capability{"ALL"}, Drop: []corev1.Capability{"ALL"}}, add: []string{"SYS_ADMIN"}, wantAdd: nil},
		{name: "add ALL drop ALL keeps individual add", declared: &corev1.Capabilities{Add: []corev1.Capability{"ALL", "CHOWN"}, Drop: []corev1.Capability{"ALL"}}, add: []string{"CHOWN"}, wantAdd: []corev1.Capability{"CHOWN"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := specWith(&corev1.SecurityContext{Capabilities: tt.declared, RunAsUser: ptr.To(int64(1000))})
			sc, reason := Render(capsRequest(tt.add...), spec, fallbackDefaults)
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
	_, reason := Render(capsRequest(), specWith(&corev1.SecurityContext{Privileged: ptr.To(true)}), fallbackDefaults)
	assert.Equal(t, "container is privileged", reason)

	req := capsRequest()
	req.Workload.Container = "missing"
	_, reason = Render(req, specWith(nil), fallbackDefaults)
	assert.Equal(t, "container not found", reason)

	// Init containers are never targeted.
	spec := &corev1.PodSpec{InitContainers: []corev1.Container{{Name: "app"}}}
	_, reason = Render(capsRequest(), spec, fallbackDefaults)
	assert.Equal(t, "container not found", reason)
}

func TestRenderReadOnlyRootFS(t *testing.T) {
	req := &Request{Control: ControlReadOnlyRootFS, Workload: Workload{Container: "app"}}
	declared := &corev1.SecurityContext{ReadOnlyRootFilesystem: ptr.To(false)}
	spec := specWith(declared)

	sc, reason := Render(req, spec, fallbackDefaults)
	assert.Empty(t, reason)
	assert.Equal(t, ptr.To(true), sc.ReadOnlyRootFilesystem, "an explicit false is overridden")
	assert.Equal(t, ptr.To(false), declared.ReadOnlyRootFilesystem, "the input is not modified")
}

func TestRenderNonRoot(t *testing.T) {
	req := &Request{Control: ControlNonRoot, Workload: Workload{Container: "app"}}

	sc, reason := Render(req, specWith(&corev1.SecurityContext{AllowPrivilegeEscalation: ptr.To(true)}), fallbackDefaults)
	assert.Empty(t, reason)
	assert.Equal(t, ptr.To(true), sc.RunAsNonRoot)
	assert.Equal(t, ptr.To(false), sc.AllowPrivilegeEscalation, "an explicit true is overridden")
	assert.Nil(t, sc.RunAsUser, "never sets runAsUser")

	_, reason = Render(req, specWith(&corev1.SecurityContext{RunAsUser: ptr.To(int64(0))}), fallbackDefaults)
	assert.Equal(t, "container runs as uid 0", reason)

	spec := specWith(nil)
	spec.SecurityContext = &corev1.PodSecurityContext{RunAsUser: ptr.To(int64(0))}
	_, reason = Render(req, spec, fallbackDefaults)
	assert.Equal(t, "container runs as uid 0", reason, "from the pod's securityContext")

	spec.Containers[1].SecurityContext = &corev1.SecurityContext{RunAsUser: ptr.To(int64(1000))}
	_, reason = Render(req, spec, fallbackDefaults)
	assert.Empty(t, reason, "the container's runAsUser wins over the pod's")
}

func TestSetsControl(t *testing.T) {
	ro := &Request{Control: ControlReadOnlyRootFS, Workload: Workload{Container: "app"}}
	nonRoot := &Request{Control: ControlNonRoot, Workload: Workload{Container: "app"}}
	podNonRoot := specWith(&corev1.SecurityContext{AllowPrivilegeEscalation: ptr.To(false)})
	podNonRoot.SecurityContext = &corev1.PodSecurityContext{RunAsNonRoot: ptr.To(true)}

	tests := []struct {
		name string
		req  *Request
		spec *corev1.PodSpec
		want bool
	}{
		{name: "nothing declared", req: capsRequest("NET_BIND_SERVICE"), spec: specWith(nil)},
		{name: "same capabilities", req: capsRequest("NET_BIND_SERVICE"), spec: specWith(&corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}, Add: []corev1.Capability{"CAP_NET_BIND_SERVICE"}}}), want: true},
		{name: "fewer capabilities", req: capsRequest("NET_BIND_SERVICE", "CHOWN"), spec: specWith(&corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}), want: true},
		{name: "another capability", req: capsRequest("NET_BIND_SERVICE"), spec: specWith(&corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}, Add: []corev1.Capability{"CHOWN"}}})},
		{name: "no drop ALL", req: capsRequest("NET_BIND_SERVICE"), spec: specWith(&corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"NET_RAW"}}})},
		{name: "read-only root fs", req: ro, spec: specWith(&corev1.SecurityContext{ReadOnlyRootFilesystem: ptr.To(true)}), want: true},
		{name: "writable root fs", req: ro, spec: specWith(&corev1.SecurityContext{ReadOnlyRootFilesystem: ptr.To(false)})},
		{name: "non-root and no escalation", req: nonRoot, spec: specWith(&corev1.SecurityContext{RunAsNonRoot: ptr.To(true), AllowPrivilegeEscalation: ptr.To(false)}), want: true},
		{name: "pod-level non-root", req: nonRoot, spec: podNonRoot, want: true},
		{name: "non-root, escalation unset", req: nonRoot, spec: specWith(&corev1.SecurityContext{RunAsNonRoot: ptr.To(true)})},
		{name: "no escalation, root allowed", req: nonRoot, spec: specWith(&corev1.SecurityContext{AllowPrivilegeEscalation: ptr.To(false)})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, setsControl(tt.req, tt.spec))
		})
	}
}

func TestRuntimeDefaults(t *testing.T) {
	docker := Defaults{Capabilities: dockerCapabilities, Exact: true}
	tests := []struct {
		name     string
		runtimes []string
		want     Defaults
	}{
		{name: "containerd", runtimes: []string{"containerd://1.7.2", "containerd://2.0.0"}, want: docker},
		{name: "docker", runtimes: []string{"docker://24.0.7"}, want: docker},
		{name: "a CRI-O node", runtimes: []string{"containerd://1.7.2", "cri-o://1.29.1"}, want: fallbackDefaults},
		{name: "an unknown runtime", runtimes: []string{"containerd://1.7.2", ""}, want: fallbackDefaults},
		{name: "no node", want: fallbackDefaults},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RuntimeDefaults(tt.runtimes))
		})
	}
	assert.Len(t, dockerCapabilities, 14)
	assert.Subset(t, dockerCapabilities, implicitCapabilities)
}

// The capabilities a staging H1 finding observed: the kernel checked NET_ADMIN
// and SYS_RESOURCE, which the container never held.
var observedCapabilities = []string{"AUDIT_WRITE", "NET_ADMIN", "SETGID", "SETUID", "SYS_RESOURCE"}

func TestRenderCapabilitiesOnContainerd(t *testing.T) {
	onContainerd := RuntimeDefaults([]string{"containerd://2.1.1"})

	sc, reason := Render(capsRequest(observedCapabilities...), specWith(nil), onContainerd)
	assert.Empty(t, reason)
	assert.Equal(t, &corev1.Capabilities{Add: []corev1.Capability{"AUDIT_WRITE", "SETGID", "SETUID"}, Drop: []corev1.Capability{"ALL"}}, sc.Capabilities,
		"the capabilities the container holds; the others are left out")

	sc, reason = Render(capsRequest("SYS_ADMIN"), specWith(&corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"SYS_ADMIN"}}}), onContainerd)
	assert.Empty(t, reason)
	assert.Equal(t, []corev1.Capability{"SYS_ADMIN"}, sc.Capabilities.Add, "a declared add is kept")
}

func TestRenderCapabilitiesWithUnknownRuntime(t *testing.T) {
	_, reason := Render(capsRequest(observedCapabilities...), specWith(nil), fallbackDefaults)
	assert.Equal(t, "capability AUDIT_WRITE is granted by default on some runtimes only", reason)

	_, reason = Render(capsRequest("NET_ADMIN"), specWith(nil), fallbackDefaults)
	assert.NotEmpty(t, reason, "CRI-O's default set is configurable")
}
