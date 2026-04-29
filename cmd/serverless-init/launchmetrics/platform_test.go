// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package launchmetrics

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
)

type stubCloudService struct {
	*cloudservice.UnknownService
	origin string
}

func (s *stubCloudService) GetOrigin() string { return s.origin }

func newStub(origin string) *stubCloudService {
	return &stubCloudService{UnknownService: &cloudservice.UnknownService{}, origin: origin}
}

func TestDetectPlatform_ManagedServicePreempts(t *testing.T) {
	for _, origin := range []string{"cloudrun", "cloudrunjobs", "containerapp", "appservice"} {
		t.Run(origin, func(t *testing.T) {
			t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "would-be-aws")
			assert.Equal(t, origin, DetectPlatform(newStub(origin)))
		})
	}
}

func TestDetectPlatform_EnvProbe(t *testing.T) {
	cs := newStub("local")

	tests := []struct {
		name     string
		envs     map[string]string
		expected string
	}{
		{"none", nil, "local"},
		{"aws lambda", map[string]string{"AWS_LAMBDA_FUNCTION_NAME": "fn"}, "aws"},
		{"aws ecs", map[string]string{"ECS_CONTAINER_METADATA_URI_V4": "http://x"}, "aws"},
		{"aws execution env", map[string]string{"AWS_EXECUTION_ENV": "AWS_ECS_FARGATE"}, "aws"},
		{"aws irsa needs both", map[string]string{"AWS_ROLE_ARN": "arn:..."}, "local"},
		{"aws irsa", map[string]string{"AWS_ROLE_ARN": "arn:...", "AWS_WEB_IDENTITY_TOKEN_FILE": "/var/run/x"}, "aws"},
		{"gcp function", map[string]string{"FUNCTION_TARGET": "main"}, "gcp"},
		{"gcp gae", map[string]string{"GAE_SERVICE": "default"}, "gcp"},
		{"azure functions", map[string]string{"AZURE_FUNCTIONS_ENVIRONMENT": "Production"}, "azure"},
		{"azure msi", map[string]string{"MSI_ENDPOINT": "http://localhost"}, "azure"},
		{"alibaba fc", map[string]string{"FC_FUNCTION_NAME": "myfn"}, "alibaba"},
		{"tencent scf", map[string]string{"SCF_FUNCTIONNAME": "myfn"}, "tencent"},
		{"oracle functions", map[string]string{"FN_FN_NAME": "myfn"}, "oracle"},
		{"ibm code engine", map[string]string{"CE_APP": "myapp"}, "ibm"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.envs {
				t.Setenv(k, v)
			}
			assert.Equal(t, tc.expected, DetectPlatform(cs))
		})
	}
}
