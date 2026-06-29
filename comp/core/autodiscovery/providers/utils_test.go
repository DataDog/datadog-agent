// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func TestSanitizeIssueIDSegment(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"kube_service://default/withErrors", "kube-service-default-witherrors"},
		{"kube_endpoint_uid://default/withErrors/", "kube-endpoint-uid-default-witherrors"},
		{"default/withErrors", "default-witherrors"},
		{"docker://abc123", "docker-abc123"},
		{"containerd://d5c06ca2-208f-444f-a7eb-6c935fa1cb47", "containerd-d5c06ca2-208f-444f-a7eb-6c935fa1cb47"},
		{"datadog-agent/pod-name (some-uid)", "datadog-agent-pod-name-some-uid"},
		{"already-valid", "already-valid"},
		{"", ""},
		{"///", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, sanitizeIssueIDSegment(tt.input))
		})
	}
}

func TestBuildStoreKey(t *testing.T) {
	res := buildStoreKey()
	assert.Equal(t, "/datadog/check_configs", res)
	res = buildStoreKey("")
	assert.Equal(t, "/datadog/check_configs", res)
	res = buildStoreKey("foo")
	assert.Equal(t, "/datadog/check_configs/foo", res)
	res = buildStoreKey("foo", "bar")
	assert.Equal(t, "/datadog/check_configs/foo/bar", res)
	res = buildStoreKey("foo", "bar", "bazz")
	assert.Equal(t, "/datadog/check_configs/foo/bar/bazz", res)
}

func TestGetPollInterval(t *testing.T) {
	cp := pkgconfigsetup.ConfigurationProviders{}
	assert.Equal(t, GetPollInterval(cp), 10*time.Second)
	cp = pkgconfigsetup.ConfigurationProviders{
		PollInterval: "foo",
	}
	assert.Equal(t, GetPollInterval(cp), 10*time.Second)
	cp = pkgconfigsetup.ConfigurationProviders{
		PollInterval: "1s",
	}
	assert.Equal(t, GetPollInterval(cp), 1*time.Second)
}
