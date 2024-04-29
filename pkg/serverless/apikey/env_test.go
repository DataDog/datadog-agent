// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package apikey

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var getFunc = func(string) (string, error) {
	return "DECRYPTED_VAL", nil
}

var mockSetSecretsFromEnv = func(t *testing.T, testEnvVars []string) {
	for envKey, envVal := range getSecretEnvVars(testEnvVars, getFunc, getFunc) {
		t.Setenv(envKey, strings.TrimSpace(envVal))
	}
}

func TestGetSecretEnvVars(t *testing.T) {
	testEnvVars := []string{
		"TEST1=VAL1",
		"TEST2=",
		"=TEST3",
		"TEST_KMS_KMS_ENCRYPTED=123",
		"TEST_SM_SECRET_ARN=123",
		"",
		"MALFORMED=ENV=VAR",
		"DD_KMS_API_KEY=123",
	}

	decryptedEnvVars := getSecretEnvVars(testEnvVars, getFunc, getFunc)

	assert.Equal(t, map[string]string{
		"TEST_KMS":   "DECRYPTED_VAL",
		"TEST_SM":    "DECRYPTED_VAL",
		apiKeyEnvVar: "DECRYPTED_VAL",
	}, decryptedEnvVars)
}

func TestDDApiKey(t *testing.T) {
	t.Setenv(apiKeyEnvVar, "abc")
	assert.NoError(t, HandleEnv())
}

func TestHasDDApiKeySecretArn(t *testing.T) {
	t.Setenv(apiKeySecretManagerEnvVar, "abc")
	mockSetSecretsFromEnv(t, os.Environ())
	assert.NoError(t, HandleEnv())
}

func TestHasDDKmsApiKeyEncrypted(t *testing.T) {
	t.Setenv(apiKeyKmsEncryptedEnvVar, "abc")
	mockSetSecretsFromEnv(t, os.Environ())
	assert.NoError(t, HandleEnv())
}

func TestHasDDKmsApiKey(t *testing.T) {
	t.Setenv(apiKeyKmsEnvVar, "abc")
	mockSetSecretsFromEnv(t, os.Environ())
	assert.NoError(t, HandleEnv())
}

func TestHasNoKeys(t *testing.T) {
	t.Setenv(apiKeyKmsEncryptedEnvVar, "")
	t.Setenv(apiKeySecretManagerEnvVar, "")
	t.Setenv(apiKeyKmsEnvVar, "")
	t.Setenv(apiKeyEnvVar, "")
	mockSetSecretsFromEnv(t, os.Environ())
	assert.Error(t, HandleEnv())
}
