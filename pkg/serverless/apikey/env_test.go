// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package apikey

import (
	"os"
	"strconv"
	"strings"
	"testing"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/stretchr/testify/assert"
)

var getFunc = func(string, aws.FIPSEndpointState) (string, error) {
	return "DECRYPTED_VAL", nil
}

var mockSetSecretsFromEnv = func(t *testing.T, testEnvVars []string) {
	pkgconfigsetup.Datadog().SetTestOnlyDynamicSchema(true)
	for envKey, envVal := range getSecretEnvVars(testEnvVars, getFunc, getFunc, false) {
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

	decryptedEnvVars := getSecretEnvVars(testEnvVars, getFunc, getFunc, false)

	assert.Equal(t, map[string]string{
		"TEST_KMS":   "DECRYPTED_VAL",
		"TEST_SM":    "DECRYPTED_VAL",
		apiKeyEnvVar: "DECRYPTED_VAL",
	}, decryptedEnvVars)
}

func TestGetSecretEnvVarsWithFIPS(t *testing.T) {
	tests := []struct {
		shouldUseFips bool
		expectedFIPS  aws.FIPSEndpointState
	}{
		{true, aws.FIPSEndpointStateEnabled},
		{false, aws.FIPSEndpointStateUnset},
	}

	for _, tc := range tests {
		t.Run(strconv.FormatBool(tc.shouldUseFips), func(t *testing.T) {
			var kmsFIPS aws.FIPSEndpointState
			var smFIPS aws.FIPSEndpointState
			mockKMSFunc := func(_ string, fips aws.FIPSEndpointState) (string, error) {
				kmsFIPS = fips
				return "decrypted", nil
			}
			mockSMFunc := func(_ string, fips aws.FIPSEndpointState) (string, error) {
				smFIPS = fips
				return "decrypted", nil
			}

			testEnvVars := []string{
				"TEST_KMS_KMS_ENCRYPTED=test",
				"TEST_SM_SECRET_ARN=test",
			}
			getSecretEnvVars(testEnvVars, mockKMSFunc, mockSMFunc, tc.shouldUseFips)

			assert.Equal(t, tc.expectedFIPS, kmsFIPS,
				"kmsFunc received wrong FIPS state")
			assert.Equal(t, tc.expectedFIPS, smFIPS,
				"smFunc received wrong FIPS state")
		})
	}
}

func TestDDApiKey(t *testing.T) {
	t.Setenv(apiKeyEnvVar, "abc")
	mockSetSecretsFromEnv(t, os.Environ())
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
