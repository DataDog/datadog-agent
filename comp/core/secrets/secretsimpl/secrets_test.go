// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsimpl

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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

	testSimpleConf = []byte(`secret_backend_arguments:
- ENC[pass1]
`)

	testSimpleConfResolved = `secret_backend_arguments:
- password1
`

	testSimpleConfOrigin = handleToContext{
		"pass1": []secretContext{
			{
				origin: "test",
				path:   []string{"secret_backend_arguments", "0"},
			},
		},
	}

	testConf = []byte(`---
instances:
- password: ENC[pass1]
  user: test
- password: ENC[pass2]
  user: test2
`)

	testConfResolved = `instances:
- password: password1
  user: test
- password: password2
  user: test2
`

	testConfOrigin = handleToContext{
		"pass1": []secretContext{
			{
				origin: "test",
				path:   []string{"instances", "0", "password"},
			},
		},
		"pass2": []secretContext{
			{
				origin: "test",
				path:   []string{"instances", "1", "password"},
			},
		},
	}

	testConfSlice = []byte(`additional_endpoints:
  http://example.com:
  - ENC[pass1]
  - data
`)

	testConfSliceResolved = `additional_endpoints:
  http://example.com:
  - password1
  - data
`

	testConfSliceOrigin = handleToContext{
		"pass1": []secretContext{
			{
				origin: "test",
				path:   []string{"additional_endpoints", "http://example.com", "0"},
			},
		},
	}

	testMultiUsageConf = []byte(`instances:
- password: ENC[pass1]
  user: test
more_endpoints:
  http://example.com:
  - ENC[pass1]
  - data
`)

	testMultiUsageConfResolved = `instances:
- password: password1
  user: test
more_endpoints:
  http://example.com:
  - password1
  - data
`

	testConfDash = []byte(`---
some_encoded_password: ENC[pass1]
keys_with_dash_string_value:
  foo: "-"
`)

	testConfResolvedDash = `keys_with_dash_string_value:
  foo: '-'
some_encoded_password: password1
`
	testConfDashOrigin = handleToContext{
		"pass1": []secretContext{
			{
				origin: "test",
				path:   []string{"some_encoded_password"},
			},
		},
	}

	testConfMultiline = []byte(`---
some_encoded_password: ENC[pass1]
`)

	testConfResolvedMultiline = `some_encoded_password: |
  password1
`
	testConfMultilineOrigin = handleToContext{
		"pass1": []secretContext{
			{
				origin: "test",
				path:   []string{"some_encoded_password"},
			},
		},
	}

	testConfNested = []byte(`---
some:
  encoded:
    data: ENC[pass1]
`)

	testConfNestedResolved = `some:
  encoded:
    data: password1
`
	testConfNestedOrigin = handleToContext{
		"pass1": []secretContext{
			{
				origin: "test",
				path:   []string{"some", "encoded", "data"},
			},
		},
	}

	testConfSibling = []byte(`
some:
  encoded:
  - data: ENC[pass1]
    sibling: text
`)

	testConfSiblingResolved = `some:
  encoded:
  - data: password1
    sibling: text
`

	testConfSiblingOrigin = handleToContext{
		"pass1": []secretContext{
			{
				origin: "test",
				// test that the path doesn't get corrupted by possible aliasing bugs in
				// the walker: if the path added to the `origin` map is not owned by the map,
				// having another string appear after the first (like "sibling"), can modify
				// this path
				path: []string{"some", "encoded", "0", "data"},
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
				origin: "test",
				path:   []string{"top_level"},
			},
		},
		"pass2": []secretContext{
			{
				origin: "test",
				path:   []string{"some", "second_level"},
			},
		},
		"pass3": []secretContext{
			{
				origin: "test",
				path:   []string{"some", "encoded", "third_level"},
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
	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	resolver := newEnabledSecretResolver(tel)
	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		return nil, fmt.Errorf("some error")
	}

	// since we didn't set any command this should return without any error
	resConf, err := resolver.Resolve(testConf, "test")
	require.NoError(t, err)
	assert.Equal(t, testConf, resConf)
}

func TestResolveSecretError(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"

	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		return nil, fmt.Errorf("some error")
	}

	_, err := resolver.Resolve(testConf, "test")
	require.NotNil(t, err)
}

func TestResolveDoestSendDuplicates(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"

	// test configuration has handle "pass1" appear twice, but fetch should only get one handle
	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		if len(secrets) > 1 {
			return nil, fmt.Errorf("duplicate handles found: %v", secrets)
		}
		return map[string]string{
			"pass1": "password1",
		}, nil
	}

	// test configuration should still resolve correctly even though handle appears more than once
	resolved, err := resolver.Resolve(testMultiUsageConf, "test")
	require.NoError(t, err)
	require.Equal(t, testMultiUsageConfResolved, string(resolved))
}

func TestResolve(t *testing.T) {
	type testCase struct {
		name                 string
		testConf             []byte
		resolvedConf         string
		expectedSecretOrigin handleToContext
		expectedScrubbedKey  []string
		secretFetchCB        func([]string) (map[string]string, error)
		secretCache          map[string]string
	}

	currentTest := t
	testCases := []testCase{
		{
			name:                 "simple",
			testConf:             testSimpleConf,
			resolvedConf:         testSimpleConfResolved,
			expectedSecretOrigin: testSimpleConfOrigin,
			expectedScrubbedKey:  []string{"secret_backend_arguments"},
			secretFetchCB: func(secrets []string) (map[string]string, error) {
				sort.Strings(secrets)
				assert.Equal(currentTest, []string{
					"pass1",
				}, secrets)

				return map[string]string{
					"pass1": "password1",
				}, nil
			},
		},
		{
			// TestResolve/map_with_dash_value checks that a nested string config value
			// that can be interpreted as YAML (such as a "-") is not interpreted as YAML by the secrets
			// decryption logic, but is left unchanged as a string instead.
			// See https://github.com/DataDog/datadog-agent/pull/6586 for details.
			name:                 "map with dash value",
			testConf:             testConfDash,
			resolvedConf:         testConfResolvedDash,
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
			// TestResolve/index_into_slice checks that when a slice contains a resolved
			// secret, the key given to the scrubber is the key of the slice, not an
			// index into that slice
			name:                 "index into slice",
			testConf:             testConfSlice,
			resolvedConf:         testConfSliceResolved,
			expectedSecretOrigin: testConfSliceOrigin,
			// NOTE: the scrubbed key is the url key, not an index into a slice
			expectedScrubbedKey: []string{"http://example.com"},
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
			resolvedConf:         testConfResolvedMultiline,
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
			resolvedConf:         testConfNestedResolved,
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
			name:                 "sibling",
			testConf:             testConfSibling,
			resolvedConf:         testConfSiblingResolved,
			expectedSecretOrigin: testConfSiblingOrigin,
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
			resolvedConf:         testConfResolved,
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
			resolvedConf:         testConfResolved,
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
			resolvedConf:         testConfResolved,
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

			tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())

			resolver := newEnabledSecretResolver(tel)
			resolver.backendCommand = "some_command"
			if tc.secretCache != nil {
				resolver.cache = tc.secretCache
			}
			resolver.fetchHookFunc = tc.secretFetchCB
			scrubbedKey := []string{}
			resolver.scrubHookFunc = func(k []string) { scrubbedKey = append(scrubbedKey, k[0]) }

			newConf, err := resolver.Resolve(tc.testConf, "test")
			require.NoError(t, err)

			assert.Equal(t, tc.resolvedConf, string(newConf))
			assert.Equal(t, tc.expectedSecretOrigin, resolver.origin)
			assert.Equal(t, tc.expectedScrubbedKey, scrubbedKey)
		})
	}
}

func TestResolveNestedWithSubscribe(t *testing.T) {
	testConf := testConfNestedMultiple

	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	resolver := newEnabledSecretResolver(tel)
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
	resolver.SubscribeToChanges(func(handle, origin string, path []string, oldValue, newValue any) {
		switch strings.Join(path, "/") {
		case "top_level":
			assert.Equal(t, "password1", newValue)
			topLevelResolved++
		case "some/second_level":
			assert.Equal(t, "password2", newValue)
			secondLevelResolved++
		case "some/encoded/third_level":
			assert.Equal(t, "password3", newValue)
			thirdLevelResolved++
		default:
			assert.Fail(t, "unknown yaml path: %s", path)
		}
	})
	_, err := resolver.Resolve(testConf, "test")

	require.NoError(t, err)
	assert.Equal(t, 1, topLevelResolved, "'top_level' secret was not resolved or resolved multiple times")
	assert.Equal(t, 1, secondLevelResolved, "'second_level' secret was not resolved or resolved multiple times")
	assert.Equal(t, 1, thirdLevelResolved, "'third_level' secret was not resolved or resolved multiple times")

	assert.Equal(t, testConfNestedOriginMultiple, resolver.origin)
}

func TestResolveCached(t *testing.T) {
	testConf := testConfNested

	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"
	resolver.cache = map[string]string{"pass1": "password1"}

	fetchHappened := 0
	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		fetchHappened++
		return map[string]string{
			"pass1": "password1",
		}, nil
	}

	totalResolved := []string{}
	resolver.SubscribeToChanges(func(handle, origin string, path []string, oldValue, newValue any) {
		totalResolved = append(totalResolved, handle)
	})
	_, err := resolver.Resolve(testConf, "test")

	// Resolve doesn't need to fetch because value is cached, but subscription is still called
	require.NoError(t, err)
	assert.Equal(t, fetchHappened, 0)
	assert.Equal(t, totalResolved, []string{"pass1"})
}

func TestResolveThenRefresh(t *testing.T) {
	testConf := testConfNestedMultiple

	// disable the allowlist for the test, let any secret changes happen
	originalAllowlistPaths := allowlistPaths
	allowlistPaths = nil
	defer func() { allowlistPaths = originalAllowlistPaths }()

	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"
	resolver.cache = map[string]string{}

	// subscribe to updates, collect list of keys that get resolved
	keysResolved := []string{}
	oldValues := []string{}
	newValues := []string{}
	resolver.SubscribeToChanges(func(handle, origin string, path []string, oldValue, newValue any) {
		keysResolved = append(keysResolved, strings.Join(path, "/"))
		oldValues = append(oldValues, fmt.Sprintf("%s", oldValue))
		newValues = append(newValues, fmt.Sprintf("%s", newValue))
	})

	// initial 3 values for these passwords
	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		return map[string]string{
			"pass1": "password1",
			"pass2": "password2",
			"pass3": "password3",
		}, nil
	}

	// resolve the secrets the first time
	_, err := resolver.Resolve(testConf, "test")
	require.NoError(t, err)
	slices.Sort(keysResolved)
	assert.Equal(t, testConfNestedOriginMultiple, resolver.origin)
	assert.Equal(t, []string{"some/encoded/third_level", "some/second_level", "top_level"}, keysResolved)

	// change the secret value of the handle 'pass2'
	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		return map[string]string{
			"pass1": "password1",
			"pass2": "update-second",
			"pass3": "password3",
		}, nil
	}

	// refresh the secrets and only collect newly updated keys
	keysResolved = []string{}
	output, err := resolver.Refresh()
	require.NoError(t, err)
	assert.Equal(t, testConfNestedOriginMultiple, resolver.origin)
	assert.Equal(t, []string{"some/second_level"}, keysResolved)
	assert.Contains(t, output, "'pass2'")
	assert.Contains(t, output, "'some/second_level'")
	assert.NotContains(t, output, "update-second")

	// change the secret values of the other two handles
	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		return map[string]string{
			"pass1": "update-first",
			"pass2": "update-second",
			"pass3": "update-third",
		}, nil
	}

	// refresh one last time and only those two handles have updated keys
	keysResolved = []string{}
	_, err = resolver.Refresh()
	require.NoError(t, err)
	slices.Sort(keysResolved)
	assert.Equal(t, testConfNestedOriginMultiple, resolver.origin)
	assert.Equal(t, []string{"some/encoded/third_level", "top_level"}, keysResolved)

	// validate the list of old and new values, sorted to make this deterministic
	sort.Strings(oldValues)
	sort.Strings(newValues)
	assert.Equal(t, []string{"", "", "", "password1", "password2", "password3"}, oldValues)
	assert.Equal(t, []string{"password1", "password2", "password3", "update-first", "update-second", "update-third"}, newValues)
}

// test that the allowlist only lets setting paths that match it get Refreshed
func TestRefreshAllowlist(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"
	resolver.cache = map[string]string{"handle": "value"}
	resolver.origin = handleToContext{
		"handle": []secretContext{
			{
				origin: "test",
				path:   []string{"another", "config", "setting"},
			},
		},
	}

	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		return map[string]string{
			"handle": "second_value",
		}, nil
	}
	changes := []string{}
	resolver.SubscribeToChanges(func(handle, origin string, path []string, oldValue, newValue any) {
		changes = append(changes, fmt.Sprintf("%s", newValue))
	})

	originalAllowlistPaths := allowlistPaths
	defer func() { allowlistPaths = originalAllowlistPaths }()

	// only allow api_key config setting to change
	allowlistPaths = map[string]struct{}{"api_key": {}}

	// Refresh means nothing changes because allowlist doesn't allow it
	_, err := resolver.Refresh()
	require.NoError(t, err)
	assert.Equal(t, changes, []string{})

	// now allow the config setting under scrutiny to change
	allowlistPaths = map[string]struct{}{"another/config/setting": {}}

	// Refresh sees the change to the handle
	_, err = resolver.Refresh()
	require.NoError(t, err)
	assert.Equal(t, changes, []string{"second_value"})
}

// test that only setting paths that match the allowlist will get notifications
// about changed secret values from a Refresh
func TestRefreshAllowlistAppliesToEachSettingPath(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"

	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		return map[string]string{
			"pass1": "password1",
		}, nil
	}

	// test configuration resolves, the secret appears at two setting paths
	resolved, err := resolver.Resolve(testMultiUsageConf, "test")
	require.NoError(t, err)
	require.Equal(t, testMultiUsageConfResolved, string(resolved))

	// set the allowlist so that only 1 of the settings matches, the 2nd does not
	originalAllowlistPaths := allowlistPaths
	allowlistPaths = map[string]struct{}{"instances/0/password": {}}
	defer func() { allowlistPaths = originalAllowlistPaths }()

	// subscribe to changes made during Refresh, keep track of updated setting paths
	changedPaths := []string{}
	resolver.SubscribeToChanges(func(handle, origin string, path []string, oldValue, newValue any) {
		changedPaths = append(changedPaths, strings.Join(path, "/"))
	})

	// the secret has a new value, will be picked up by next Refresh
	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		return map[string]string{
			"pass1": "second_password",
		}, nil
	}

	// only 1 setting path got updated
	_, err = resolver.Refresh()
	require.NoError(t, err)
	assert.Equal(t, changedPaths, []string{"instances/0/password"})
}

// test that adding to the audit file stops working when the file gets too large
func TestRefreshAddsToAuditFile(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "")
	assert.NoError(t, err)

	originalAllowlistPaths := allowlistPaths
	allowlistPaths = map[string]struct{}{"another/config/setting": {}}
	defer func() { allowlistPaths = originalAllowlistPaths }()

	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"
	resolver.cache = map[string]string{"handle": "value"}
	resolver.origin = handleToContext{
		"handle": []secretContext{
			{
				origin: "test",
				path:   []string{"another", "config", "setting"},
			},
		},
	}
	resolver.auditFilename = tmpfile.Name()
	resolver.auditFileMaxSize = 1000 // enough to add a few rows

	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		return map[string]string{
			"handle": "second_value",
		}, nil
	}

	// Refresh the secrets, which will add to the audit file
	_, err = resolver.Refresh()
	require.NoError(t, err)
	assert.Equal(t, auditFileNumRows(tmpfile.Name()), 1)

	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		return map[string]string{
			"handle": "third_value",
		}, nil
	}

	// Refresh secrets again, which will add another row the audit file
	_, err = resolver.Refresh()
	require.NoError(t, err)
	assert.Equal(t, auditFileNumRows(tmpfile.Name()), 2)

	resolver.fetchHookFunc = func(secrets []string) (map[string]string, error) {
		return map[string]string{
			"handle": "fourth_value",
		}, nil
	}
}

// helper to read number of rows in the audit file
func auditFileNumRows(filename string) int {
	data, _ := os.ReadFile(filename)
	return len(strings.Split(strings.Trim(string(data), "\n"), "\n"))
}

func TestIsLikelyAPIOrAppKey(t *testing.T) {
	type testCase struct {
		name        string
		handle      string
		secretValue string
		origin      handleToContext
		expect      bool
	}

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
			result := isLikelyAPIOrAppKey(tc.handle, tc.secretValue, tc.origin)
			assert.Equal(t, tc.expect, result)
		})
	}
}
