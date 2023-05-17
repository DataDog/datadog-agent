// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets

package secrets

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

var (
	testYamlHash = []byte(`
slice:
  - "1"
  - [test1, test2]
  - 123
hash:
  a: test3
  b: "2"
  c: 456
  slice:
    - test4
    - test5
`)

	testYamlHashUpdated = []byte(`hash:
  a: test3_verified
  b: 2_verified
  c: 456
  slice:
  - test4_verified
  - test5_verified
slice:
- 1_verified
- - test1_verified
  - test2_verified
- 123
`)

	testConf = []byte(`---
instances:
- password: ENC[pass1]
  user: test
- password: ENC[pass2]
  user: test2
`)

	testConfDecrypted = `instances:
- password: password1
  user: test
- password: password2
  user: test2
`

	testConfOrigin = handleToContext{
		"pass1": []secretContext{
			{
				origin:   "test",
				yamlPath: "instances/password",
			},
		},
		"pass2": []secretContext{
			{
				origin:   "test",
				yamlPath: "instances/password",
			},
		},
	}

	testConfDash = []byte(`---
some_encoded_password: ENC[pass1]
keys_with_dash_string_value:
  foo: "-"
`)

	testConfDecryptedDash = `keys_with_dash_string_value:
  foo: '-'
some_encoded_password: password1
`
	testConfDashOrigin = handleToContext{
		"pass1": []secretContext{
			{
				origin:   "test",
				yamlPath: "some_encoded_password",
			},
		},
	}

	testConfMultiline = []byte(`---
some_encoded_password: ENC[pass1]
`)

	testConfDecryptedMultiline = `some_encoded_password: |
  password1
`
	testConfMultilineOrigin = handleToContext{
		"pass1": []secretContext{
			{
				origin:   "test",
				yamlPath: "some_encoded_password",
			},
		},
	}

	testConfNested = []byte(`---
some:
  encoded:
    data: ENC[pass1]
`)

	testConfDecryptedNested = `some:
  encoded:
    data: password1
`
	testConfNestedOrigin = handleToContext{
		"pass1": []secretContext{
			{
				origin:   "test",
				yamlPath: "some/encoded/data",
			},
		},
	}
)

func resetPackageVars() {
	secretBackendCommand = ""
	secretBackendArguments = []string{}
	secretCache = map[string]string{}
	secretOrigin = make(handleToContext)
	secretFetcher = fetchSecret
	secretBackendTimeout = 0
	scrubberAddReplacer = scrubber.AddStrippedKeys
	removeTrailingLinebreak = false
}

func TestIsEnc(t *testing.T) {
	enc, secret := isEnc("")
	assert.False(t, enc)
	assert.Equal(t, "", secret)

	enc, secret = isEnc("ENC[]")
	assert.True(t, enc)
	assert.Equal(t, "", secret)

	enc, _ = isEnc("test")
	assert.False(t, enc)

	enc, _ = isEnc("ENC[")
	assert.False(t, enc)

	enc, secret = isEnc("ENC[test]")
	assert.True(t, enc)
	assert.Equal(t, "test", secret)

	enc, secret = isEnc("ENC[]]]]")
	assert.True(t, enc)
	assert.Equal(t, "]]]", secret)

	enc, secret = isEnc("  ENC[test]	")
	assert.True(t, enc)
	assert.Equal(t, "test", secret)
}

func TestWalkerError(t *testing.T) {
	var config interface{}
	err := yaml.Unmarshal(testYamlHash, &config)
	require.NoError(t, err)

	err = walk(&config, nil, func([]string, string) (string, error) {
		return "", fmt.Errorf("some error")
	})
	assert.NotNil(t, err)
}

func TestWalkerSimple(t *testing.T) {
	var config interface{}
	err := yaml.Unmarshal([]byte("test"), &config)
	require.NoError(t, err)

	stringsCollected := []string{}
	err = walk(&config, nil, func(_ []string, str string) (string, error) {
		stringsCollected = append(stringsCollected, str)
		return str + "_verified", nil
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"test"}, stringsCollected)

	updatedConf, err := yaml.Marshal(config)
	require.NoError(t, err)
	assert.Equal(t, string("test_verified\n"), string(updatedConf))
}

func TestWalkerComplex(t *testing.T) {
	var config interface{}
	err := yaml.Unmarshal(testYamlHash, &config)
	require.NoError(t, err)

	stringsCollected := []string{}
	err = walk(&config, nil, func(_ []string, str string) (string, error) {
		stringsCollected = append(stringsCollected, str)
		return str + "_verified", nil
	})
	require.NoError(t, err)

	sort.Strings(stringsCollected)
	assert.Equal(t, []string{
		"1",
		"2",
		"test1",
		"test2",
		"test3",
		"test4",
		"test5",
	}, stringsCollected)

	updatedConf, err := yaml.Marshal(config)
	require.NoError(t, err)
	assert.Equal(t, string(testYamlHashUpdated), string(updatedConf))
}

func TestDecryptNoCommand(t *testing.T) {
	defer resetPackageVars()
	secretFetcher = func(secrets []string) (map[string]string, error) {
		return nil, fmt.Errorf("some error")
	}

	// since we didn't set any command this should return without any error
	resConf, err := Decrypt(testConf, "test")
	require.NoError(t, err)
	assert.Equal(t, testConf, resConf)
}

func TestDecryptSecretError(t *testing.T) {
	secretBackendCommand = "some_command"
	defer resetPackageVars()

	secretFetcher = func(secrets []string) (map[string]string, error) {
		return nil, fmt.Errorf("some error")
	}

	_, err := Decrypt(testConf, "test")
	require.NotNil(t, err)
}

func TestDecrypt(t *testing.T) {
	type testCase struct {
		name                 string
		testConf             []byte
		decryptedConf        string
		expectedSecretOrigin handleToContext
		expectedScrubbedKey  []string
		secretFetchCB        func([]string) (map[string]string, error)
		secretCache          map[string]string
	}

	currentTest := t
	testCases := []testCase{
		{
			// TestDecryptSecretStringMapStringWithDashValue checks that a nested string config value
			// that can be interpreted as YAML (such as a "-") is not interpreted as YAML by the secrets
			// decryption logic, but is left unchanged as a string instead.
			// See https://github.com/DataDog/datadog-agent/pull/6586 for details.
			name:                 "map with dash value",
			testConf:             testConfDash,
			decryptedConf:        testConfDecryptedDash,
			expectedSecretOrigin: testConfDashOrigin,
			expectedScrubbedKey:  []string{"some_encoded_password"},
			secretFetchCB: func(secrets []string) (map[string]string, error) {
				assert.Equal(currentTest, []string{
					"pass1",
				}, secrets)

				return map[string]string{
					"pass1": "password1",
				}, nil
			},
		},
		{
			name:                 "multiline",
			testConf:             testConfMultiline,
			decryptedConf:        testConfDecryptedMultiline,
			expectedSecretOrigin: testConfMultilineOrigin,
			expectedScrubbedKey:  []string{"some_encoded_password"},
			secretFetchCB: func(secrets []string) (map[string]string, error) {
				assert.Equal(currentTest, []string{
					"pass1",
				}, secrets)

				return map[string]string{
					"pass1": "password1\n",
				}, nil
			},
		},
		{
			name:                 "nested",
			testConf:             testConfNested,
			decryptedConf:        testConfDecryptedNested,
			expectedSecretOrigin: testConfNestedOrigin,
			expectedScrubbedKey:  []string{"data"},
			secretFetchCB: func(secrets []string) (map[string]string, error) {
				assert.Equal(currentTest, []string{
					"pass1",
				}, secrets)

				return map[string]string{
					"pass1": "password1",
				}, nil
			},
		},
		{
			name:                 "no cache",
			testConf:             testConf,
			decryptedConf:        testConfDecrypted,
			expectedSecretOrigin: testConfOrigin,
			expectedScrubbedKey:  []string{"password", "password"},
			secretFetchCB: func(secrets []string) (map[string]string, error) {
				sort.Strings(secrets)
				assert.Equal(currentTest, []string{
					"pass1",
					"pass2",
				}, secrets)

				return map[string]string{
					"pass1": "password1",
					"pass2": "password2",
				}, nil
			},
		},
		{
			name:                 "partial cache",
			testConf:             testConf,
			decryptedConf:        testConfDecrypted,
			expectedSecretOrigin: testConfOrigin,
			expectedScrubbedKey:  []string{"password", "password"},
			secretCache:          map[string]string{"pass1": "password1"},
			secretFetchCB: func(secrets []string) (map[string]string, error) {
				sort.Strings(secrets)
				assert.Equal(currentTest, []string{
					"pass2",
				}, secrets)

				return map[string]string{
					"pass2": "password2",
				}, nil
			},
		},
		{
			name:                 "full cache",
			testConf:             testConf,
			decryptedConf:        testConfDecrypted,
			expectedSecretOrigin: testConfOrigin,
			expectedScrubbedKey:  []string{"password", "password"},
			secretCache:          map[string]string{"pass1": "password1", "pass2": "password2"},
			secretFetchCB: func(secrets []string) (map[string]string, error) {
				require.Fail(currentTest, "Secret Cache was not used properly")
				return nil, nil
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentTest = t
			t.Cleanup(resetPackageVars)

			secretBackendCommand = "some_command"
			if tc.secretCache != nil {
				secretCache = tc.secretCache
			}
			secretFetcher = tc.secretFetchCB
			scrubbedKey := []string{}
			scrubberAddReplacer = func(k []string) { scrubbedKey = append(scrubbedKey, k[0]) }

			newConf, err := Decrypt(tc.testConf, "test")
			require.NoError(t, err)

			assert.Equal(t, tc.decryptedConf, string(newConf))
			assert.Equal(t, tc.expectedSecretOrigin, secretOrigin)
			assert.Equal(t, tc.expectedScrubbedKey, scrubbedKey)
		})
	}
}
