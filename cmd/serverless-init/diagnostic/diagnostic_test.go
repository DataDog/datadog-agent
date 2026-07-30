// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package diagnostic

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	"github.com/stretchr/testify/assert"
)

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "masks DD_API_KEY",
			input: "DD_API_KEY=abc123",
			want:  "DD_API_KEY=***",
		},
		{
			name:  "masks TOKEN",
			input: "MY_TOKEN=secret-value",
			want:  "MY_TOKEN=***",
		},
		{
			name:  "masks SECRET",
			input: "DB_SECRET=pass",
			want:  "DB_SECRET=***",
		},
		{
			name:  "masks PASSWORD",
			input: "ROOT_PASSWORD=hunter2",
			want:  "ROOT_PASSWORD=***",
		},
		{
			name:  "passes DD_SITE",
			input: "DD_SITE=datadoghq.com",
			want:  "DD_SITE=datadoghq.com",
		},
		{
			name:  "passes K_SERVICE",
			input: "K_SERVICE=my-cloud-run-service",
			want:  "K_SERVICE=my-cloud-run-service",
		},
		{
			name:  "passes K_REVISION",
			input: "K_REVISION=my-cloud-run-service-00001-abc",
			want:  "K_REVISION=my-cloud-run-service-00001-abc",
		},
		{
			name:  "passes CONTAINER_APP_NAME",
			input: "CONTAINER_APP_NAME=my-app",
			want:  "CONTAINER_APP_NAME=my-app",
		},
		{
			name:  "passes WEBSITE_SITE_NAME",
			input: "WEBSITE_SITE_NAME=my-web-app",
			want:  "WEBSITE_SITE_NAME=my-web-app",
		},
		{
			name:  "handles no equals sign",
			input: "MALFORMED",
			want:  "MALFORMED",
		},
		{
			name:  "case-insensitive key matching",
			input: "my_api_key=value",
			want:  "my_api_key=***",
		},
		{
			name:  "preserves value with embedded equals sign",
			input: "DD_SITE=us1.datadoghq.com",
			want:  "DD_SITE=us1.datadoghq.com",
		},
		{
			name:  "preserves empty value",
			input: "K_SERVICE=",
			want:  "K_SERVICE=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, maskSecret(tt.input))
		})
	}
}

// TestLogIfEnabled_NoopWhenDisabled verifies that LogIfEnabled is a no-op
// when DD_SERVERLESS_DIAGNOSTIC_INFO is not "true". We pass nil for cloudService
// to ensure the function exits before dereferencing it — a nil deref here
// would mean the guard is broken.
func TestLogIfEnabled_NoopWhenDisabled(t *testing.T) {
	for _, val := range []string{"", "false", "FALSE", "0", "yes"} {
		t.Run("env="+val, func(t *testing.T) {
			t.Setenv(diagnosticEnvVar, val)
			// Must not panic — cs.GetOrigin() is never reached when disabled.
			assert.NotPanics(t, func() {
				LogIfEnabled(mode.Conf{}, nil)
			})
		})
	}
}
