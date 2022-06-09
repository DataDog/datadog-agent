package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSecretEnvVars(t *testing.T) {
	var getFunc = func(string) (string, error) {
		return "DECRYPTED_VAL", nil
	}
	testEnvVars := []string{
		"TEST1=VAL1",
		"TEST2=",
		"=TEST3",
		"TEST_KMS_KMS_ENCRYPTED=123",
		"TEST_SM_SECRET_ARN=123",
		"",
		"MALFORMED=ENV=VAR",
	}

	decryptedEnvVars := getSecretEnvVars(testEnvVars, getFunc, getFunc)

	assert.Equal(t, map[string]string{
		"TEST_KMS": "DECRYPTED_VAL",
		"TEST_SM":  "DECRYPTED_VAL",
	}, decryptedEnvVars)
}
