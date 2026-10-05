// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_customresources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestResourcePolicyCompatibilityMode(t *testing.T) {
	tests := []struct {
		name     string
		group    string
		version  string
		resource string
		want     schema.GroupVersionResource
		wantErr  string
	}{
		{
			name:     "ordinary CRD",
			group:    "cert-manager.io",
			version:  "v1",
			resource: "certificates",
			want:     schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
		},
		{name: "core resource", group: "", version: "v1", resource: "secrets", wantErr: "requires an explicit entry"},
		{name: "legacy native group", group: "apps", version: "v1", resource: "deployments", wantErr: "native Kubernetes API group"},
		{name: "reserved k8s group", group: "admissionregistration.k8s.io", version: "v1", resource: "mutatingwebhookconfigurations", wantErr: "native Kubernetes API group"},
		{name: "reserved kubernetes group", group: "example.kubernetes.io", version: "v1", resource: "widgets", wantErr: "native Kubernetes API group"},
		{name: "subresource smuggling", group: "example.com", version: "v1", resource: "pods/example/exec", wantErr: "invalid Kubernetes resource resource"},
		{name: "invalid group path", group: "example.com%2fapis", version: "v1", resource: "widgets", wantErr: "invalid Kubernetes resource group"},
		{name: "empty version", group: "example.com", version: "", resource: "widgets", wantErr: "invalid Kubernetes resource version"},
		{name: "empty resource", group: "example.com", version: "v1", resource: "", wantErr: "invalid Kubernetes resource resource"},
	}

	policy := newResourcePolicy(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := policy.groupVersionResource(tt.group, tt.version, tt.resource)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResourcePolicyExactAllowlist(t *testing.T) {
	t.Run("allows exact CRD", func(t *testing.T) {
		policy := newResourcePolicy([]string{"cert-manager.io/v1/certificates"})
		got, err := policy.groupVersionResource("cert-manager.io", "v1", "certificates")
		require.NoError(t, err)
		assert.Equal(t, schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}, got)
	})

	t.Run("denies unlisted CRD", func(t *testing.T) {
		policy := newResourcePolicy([]string{"cert-manager.io/v1/certificates"})
		_, err := policy.groupVersionResource("argoproj.io", "v1alpha1", "applications")
		require.ErrorContains(t, err, "is not in private_action_runner.kubernetes_allowed_custom_resources")
	})

	t.Run("explicit empty list denies all", func(t *testing.T) {
		policy := newResourcePolicy([]string{})
		_, err := policy.groupVersionResource("cert-manager.io", "v1", "certificates")
		require.ErrorContains(t, err, "is not in private_action_runner.kubernetes_allowed_custom_resources")
	})

	t.Run("explicit native group override", func(t *testing.T) {
		policy := newResourcePolicy([]string{"gateway.networking.k8s.io/v1/gateways"})
		_, err := policy.groupVersionResource("gateway.networking.k8s.io", "v1", "gateways")
		require.NoError(t, err)
	})

	t.Run("explicit core resource override", func(t *testing.T) {
		policy := newResourcePolicy([]string{"v1/secrets"})
		got, err := policy.groupVersionResource("", "v1", "secrets")
		require.NoError(t, err)
		assert.Equal(t, schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, got)
	})

	t.Run("invalid path cannot be overridden", func(t *testing.T) {
		policy := newResourcePolicy([]string{"example.com/v1/pods/example/exec"})
		_, err := policy.groupVersionResource("example.com", "v1", "pods/example/exec")
		require.ErrorContains(t, err, "invalid Kubernetes resource resource")
	})
}
