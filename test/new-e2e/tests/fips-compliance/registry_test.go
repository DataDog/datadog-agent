// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fipscompliance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppsRegistryFromImage(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  string
	}{
		{"internal registry", "669783387624.dkr.ecr.us-east-1.amazonaws.com/ci/datadog-agent/apps-fips-server:v0.0.10", "669783387624.dkr.ecr.us-east-1.amazonaws.com/ci/datadog-agent"},
		{"default registry", "ghcr.io/datadog/apps-fips-server:v0.0.10", "ghcr.io/datadog"},
		{"digest reference", "ghcr.io/datadog/apps-fips-server@sha256:abc", "ghcr.io/datadog"},
		{"no tag", "ghcr.io/datadog/apps-fips-server", "ghcr.io/datadog"},
		{"trailing newline", "ghcr.io/datadog/apps-fips-server:v0.0.10\n", "ghcr.io/datadog"},
		{"quoted", "'ghcr.io/datadog/apps-fips-server:v0.0.10'", "ghcr.io/datadog"},
		{"no registry prefix", "apps-fips-server:v0.0.10", ""},
		{"different repository with the same prefix", "ghcr.io/datadog/apps-fips-server-other:v1", ""},
		{"other image", "ghcr.io/datadog/apps-tracegen:v1", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, appsRegistryFromImage(tt.image))
		})
	}
}

func TestComposeEnv(t *testing.T) {
	s := fipsServer{
		extraEnv:     map[string]string{"CLUSTER_AGENT_IMAGE": "img", "CERT": "from-extra-env"},
		appsRegistry: "registry.example/apps",
	}

	assert.Equal(t, map[string]string{
		"CLUSTER_AGENT_IMAGE": "img",
		"CERT":                "from-extra-env",
		"DD_APPS_REGISTRY":    "registry.example/apps",
	}, s.composeEnv(nil))

	assert.Equal(t, "rsa", s.composeEnv(map[string]string{"CERT": "rsa"})["CERT"], "per-call env overrides extraEnv")

	s.appsRegistry = ""
	assert.NotContains(t, s.composeEnv(nil), "DD_APPS_REGISTRY", "no registry found keeps the fixture default")
}

func TestComposeService(t *testing.T) {
	s := fipsServer{}
	assert.Equal(t, "fips-server", s.composeService())
	s.service = "dd-fips-server"
	assert.Equal(t, "dd-fips-server", s.composeService())
}
