// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsimpl

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	testConfResolveed = `instances:
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

	testConfResolveedDash = `keys_with_dash_string_value:
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

	testConfResolveedMultiline = `some_encoded_password: |
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

	testConfResolveedNested = `some:
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

	testConfNestedMultiple = []byte(`---
top_level: ENC[pass1]
some:
  second_level: ENC[pass2]
  encoded:
    third_level: ENC[pass3]
`)

	testConfNestedOriginMultiple = handleToContext{
		"pass1": []secretContext{
			{
				origin:   "test",
				yamlPath: "top_level",
			},
		},
		"pass2": []secretContext{
			{
				origin:   "test",
				yamlPath: "some/second_level",
			},
		},
		"pass3": []secretContext{
			{
				origin:   "test",
				yamlPath: "some/encoded/third_level",
			},
		},
	}
)

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

func TestResolveNoCommand(t *testing.T) {
	resolver := newEnabledSecretResolver()
	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		return nil, fmt.Errorf("some error")
	}

	// since we didn't set any command this should return without any error
	resConf, err := resolver.Resolve(testConf, "test")
	require.NoError(t, err)
	assert.Equal(t, testConf, resConf)
}

func TestResolveSecretError(t *testing.T) {
	resolver := newEnabledSecretResolver()
	resolver.backendCommand = "some_command"

	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		return nil, fmt.Errorf("some error")
	}

	_, err := resolver.Resolve(testConf, "test")
	require.NotNil(t, err)
}

func TestResolve(t *testing.T) {
	type testCase struct {
		name                 string
		testConf             []byte
		resolveedConf        string
		expectedSecretOrigin handleToContext
		expectedScrubbedKey  []string
		secretFetchCB        func([]string) (map[string]string, error)
		secretCache          map[string]string
	}

	currentTest := t
	testCases := []testCase{
		{
			// TestResolveSecretStringMapStringWithDashValue checks that a nested string config value
			// that can be interpreted as YAML (such as a "-") is not interpreted as YAML by the secrets
			// decryption logic, but is left unchanged as a string instead.
			// See https://github.com/DataDog/datadog-agent/pull/6586 for details.
			name:                 "map with dash value",
			testConf:             testConfDash,
			resolveedConf:        testConfResolveedDash,
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
			resolveedConf:        testConfResolveedMultiline,
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
			resolveedConf:        testConfResolveedNested,
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
			resolveedConf:        testConfResolveed,
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
			resolveedConf:        testConfResolveed,
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
			resolveedConf:        testConfResolveed,
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

			resolver := newEnabledSecretResolver()
			resolver.backendCommand = "some_command"
			if tc.secretCache != nil {
				resolver.cache = tc.secretCache
			}
			resolver.fetchHookFunc = tc.secretFetchCB
			scrubbedKey := []string{}
			resolver.scrubHookFunc = func(k []string) { scrubbedKey = append(scrubbedKey, k[0]) }

			newConf, err := resolver.Resolve(tc.testConf, "test")
			require.NoError(t, err)

			assert.Equal(t, tc.resolveedConf, string(newConf))
			assert.Equal(t, tc.expectedSecretOrigin, resolver.origin)
			assert.Equal(t, tc.expectedScrubbedKey, scrubbedKey)
		})
	}
}

func TestResolveWithCallback(t *testing.T) {
	testConf := testConfNestedMultiple

	resolver := newEnabledSecretResolver()
	resolver.backendCommand = "some_command"
	resolver.cache = map[string]string{"pass3": "password3"}

	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		return map[string]string{
			"pass1": "password1",
			"pass2": "password2",
		}, nil
	}

	topLevelResolved := 0
	secondLevelResolved := 0
	thirdLevelResolved := 0
	err := resolver.ResolveWithCallback(
		testConf,
		"test",
		func(yamlPath []string, value any) bool {
			switch strings.Join(yamlPath, "/") {
			case "top_level":
				assert.Equal(t, "password1", value)
				topLevelResolved++
			case "some/second_level":
				assert.Equal(t, "password2", value)
				secondLevelResolved++
			case "some/encoded/third_level":
				assert.Equal(t, "password3", value)
				thirdLevelResolved++
			default:
				assert.Fail(t, "unknown yaml path: %s", yamlPath)
			}
			return true
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, topLevelResolved, "'top_level' secret was not resolved or resolved multiple times")
	assert.Equal(t, 1, secondLevelResolved, "'second_level' secret was not resolved or resolved multiple times")
	assert.Equal(t, 1, thirdLevelResolved, "'third_level' secret was not resolved or resolved multiple times")

	assert.Equal(t, testConfNestedOriginMultiple, resolver.origin)
}
