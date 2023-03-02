// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startchecker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDDApiKey(t *testing.T) {
	t.Setenv("DD_API_KEY", "abc")
	rule := &ApiKeyEnvRule{}
	assert.True(t, rule.ok())
}

func TestDDApiKeySecretArn(t *testing.T) {
	t.Setenv("DD_API_KEY_SECRET_ARN", "abc")
	rule := &ApiKeyEnvRule{}
	assert.True(t, rule.ok())
}

func TestDDKmsApiKey(t *testing.T) {
	t.Setenv("DD_KMS_API_KEY", "abc")
	rule := &ApiKeyEnvRule{}
	assert.True(t, rule.ok())
}

func TestNoKeys(t *testing.T) {
	t.Setenv("DD_KMS_API_KEY", "")
	t.Setenv("DD_API_KEY_SECRET_ARN", "")
	t.Setenv("DD_API_KEY", "")
	rule := &ApiKeyEnvRule{}
	assert.False(t, rule.ok())
}
