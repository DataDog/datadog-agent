// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package apikey

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var getFunc = func(string, aws.FIPSEndpointState) (string, error) {
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

func TestGetSecretEnvVarsWithFIPSEndpoint(t *testing.T) {
	tests := []struct {
		region       string
		expectedFIPS aws.FIPSEndpointState
	}{
		{"us-gov-west-1", aws.FIPSEndpointStateEnabled},
		{"us-west-2", aws.FIPSEndpointStateUnset},
	}

	for _, tc := range tests {
		t.Run(tc.region, func(t *testing.T) {
			os.Setenv(lambdaRegionEnvVar, tc.region)

			var kmsFIPS aws.FIPSEndpointState
			var smFIPS aws.FIPSEndpointState
			mockKMSFunc := func(val string, fips aws.FIPSEndpointState) (string, error) {
				kmsFIPS = fips
				return "decrypted", nil
			}
			mockSMFunc := func(val string, fips aws.FIPSEndpointState) (string, error) {
				smFIPS = fips
				return "decrypted", nil
			}

			testEnvVars := []string{
				"TEST_KMS_KMS_ENCRYPTED=test",
				"TEST_SM_SECRET_ARN=test",
			}
			getSecretEnvVars(testEnvVars, mockKMSFunc, mockSMFunc)

			assert.Equal(t, tc.expectedFIPS, kmsFIPS,
				"kmsFunc received wrong FIPS state for region %s", tc.region)
			assert.Equal(t, tc.expectedFIPS, smFIPS,
				"smFunc received wrong FIPS state for region %s", tc.region)
		})
	}
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
