// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secretsimpl is the implementation for the secrets component
package secretsimpl

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func setupTest(t *testing.T) (string, func()) {
	// create a temporary file and use it as the refresh audit file
	tmpFile, err := os.CreateTemp("", "*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	// create a known timestamp for tests (chosen from stackoverflow)
	originalTimeNow := timeNowFunc
	timeNowFunc = func() time.Time {
		return time.Unix(1405544146, 0)
	}
	// instead of 90 days, set the timeLimit to 90 years
	originalCutoffLimit := cutoffLimit
	cutoffLimit = time.Hour * 24 * 365 * 90
	// func to cleanup when the test is done
	cleanupFunc := func() {
		cutoffLimit = originalCutoffLimit
		timeNowFunc = originalTimeNow
		os.Remove(tmpFile.Name())
	}
	return tmpFile.Name(), cleanupFunc
}

func TestAddSecretsToNewFile(t *testing.T) {
	tmpFileName, cleanupFunc := setupTest(t)
	defer cleanupFunc()

	secretResponse := map[string]string{
		"pw1": "password1",
		"pw2": "password2",
	}

	addToAuditFile(tmpFileName, secretResponse, nil, 1000000)

	data, err := os.ReadFile(tmpFileName)
	if err != nil {
		t.Fatal(err)
	}
	expect := `[
  {
    "when": "2014-07-16T20:55:46Z",
    "handle": "pw1"
  },
  {
    "when": "2014-07-16T20:55:46Z",
    "handle": "pw2"
  }
]`
	assert.Equal(t, expect, string(data))
}

func TestAddSecretsToExistingFile(t *testing.T) {
	tmpFileName, cleanupFunc := setupTest(t)
	defer cleanupFunc()

	startContent := `[
  {
    "when": "2014-07-14T20:55:46Z",
    "handle": "pw1"
  }
]`
	os.WriteFile(tmpFileName, []byte(startContent), 0644)

	secretResponse := map[string]string{
		"pw2": "password2",
	}

	addToAuditFile(tmpFileName, secretResponse, nil, 1000000)

	data, err := os.ReadFile(tmpFileName)
	if err != nil {
		t.Fatal(err)
	}
	expect := `[
  {
    "when": "2014-07-14T20:55:46Z",
    "handle": "pw1"
  },
  {
    "when": "2014-07-16T20:55:46Z",
    "handle": "pw2"
  }
]`
	assert.Equal(t, expect, string(data))
}

func TestAddAPIKeyToNewFile(t *testing.T) {
	tmpFileName, cleanupFunc := setupTest(t)
	defer cleanupFunc()

	secretResponse := map[string]string{
		"api_key": "0123456789abcdef0123456789abcdef",
	}
	origin := map[string][]secretContext{
		"api_key": {
			{
				origin: "conf.yaml",
				path:   []string{"api_key"},
			},
		},
	}

	addToAuditFile(tmpFileName, secretResponse, origin, 1000000)

	data, err := os.ReadFile(tmpFileName)
	if err != nil {
		t.Fatal(err)
	}
	expect := `[
  {
    "when": "2014-07-16T20:55:46Z",
    "handle": "api_key",
    "value": "***************************bcdef"
  }
]`
	assert.Equal(t, expect, string(data))
}

func TestAddSecretAndRemoveOldEntries(t *testing.T) {
	tmpFileName, cleanupFunc := setupTest(t)
	defer cleanupFunc()

	startContent := `[
  {
    "when": "2014-07-13T20:55:46Z",
    "handle": "pw1"
  },
  {
    "when": "2014-07-15T20:55:46Z",
    "handle": "pw2"
  }
]`
	os.WriteFile(tmpFileName, []byte(startContent), 0644)

	// set cuttoffLimit to now minus 2014/7/14
	// that means we will drop the first entry but not the second
	dt, err := time.Parse(time.RFC3339, "2014-07-14T20:55:46Z")
	if err != nil {
		t.Fatal(err)
	}
	cutoffLimit = time.Since(dt)

	secretResponse := map[string]string{
		"pw3": "password3",
	}

	addToAuditFile(tmpFileName, secretResponse, nil, 1000000)

	data, err := os.ReadFile(tmpFileName)
	if err != nil {
		t.Fatal(err)
	}
	expect := `[
  {
    "when": "2014-07-15T20:55:46Z",
    "handle": "pw2"
  },
  {
    "when": "2014-07-16T20:55:46Z",
    "handle": "pw3"
  }
]`
	assert.Equal(t, expect, string(data))
}

func TestIsLikelyAPIOrAppKey(t *testing.T) {
	type testCase struct {
		name        string
		handle      string
		secretValue string
		origin      handleToContext
		expect      bool
	}

	currentTest := t
	testCases := []testCase{
		{
			name:        "looks like an api_key",
			handle:      "vault://userToken",
			secretValue: "0123456789abcdef0123456789abcdef",
			origin: map[string][]secretContext{
				"vault://userToken": {
					{
						origin: "conf.yaml",
						path:   []string{"provider", "credentials", "apiKey"},
					},
				},
			},
			expect: true,
		},
		{
			name:        "wrong length",
			handle:      "vault://userToken",
			secretValue: "0123456789abcdef0123456789abc",
			origin: map[string][]secretContext{
				"vault://userToken": {
					{
						origin: "conf.yaml",
						path:   []string{"provider", "credentials", "apiKey"},
					},
				},
			},
			expect: false,
		},
		{
			name:        "not hex",
			handle:      "vault://userToken",
			secretValue: "0123456789stuvwx0123456789stuvwx",
			origin: map[string][]secretContext{
				"vault://userToken": {
					{
						origin: "conf.yaml",
						path:   []string{"provider", "credentials", "apiKey"},
					},
				},
			},
			expect: false,
		},
		{
			name:        "likely password",
			handle:      "vault://secretPassword",
			secretValue: "0123456789abcdef0123456789abcdef",
			origin: map[string][]secretContext{
				"vault://userToken": {
					{
						origin: "conf.yaml",
						path:   []string{"provider", "credentials", "password"},
					},
				},
			},
			expect: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentTest = t
			result := isLikelyAPIOrAppKey(tc.handle, tc.secretValue, tc.origin)
			assert.Equal(currentTest, tc.expect, result)
		})
	}
}
