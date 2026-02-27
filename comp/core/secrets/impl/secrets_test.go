// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsimpl

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
)

var (
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

	testSecretFiltering = []byte(`instances:
- some_obj:
  - ENC[non_k8s_value]
  - ENC[k8s_secret@namespace1/sec1/key1]
  - ENC[k8s_secret@default/sec1/key1]
`)
)

func newResolver(_ *testing.T, params secrets.ConfigParams) *secretResolver {
	resolver := NewComponent(
		Requires{
			Telemetry: nooptelemetry.GetCompatComponent(),
		},
	).Comp.(*secretResolver)

	resolver.Configure(params)
	return resolver
}

func TestResolveNoCommand(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return nil, errors.New("some error")
	}

	// since we didn't set any command this should return without any error
	resConf, err := resolver.Resolve(testConf, "test", "", "", true)
	require.NoError(t, err)
	assert.Equal(t, testConf, resConf)
}

func TestResolveSecretError(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"

	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return nil, errors.New("some error")
	}

	_, err := resolver.Resolve(testConf, "test", "", "", true)
	require.NotNil(t, err)
}

func TestResolveDoestSendDuplicates(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
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
	resolved, err := resolver.Resolve(testMultiUsageConf, "test", "", "", true)
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
			secretFetchCB: func(_ []string) (map[string]string, error) {
				require.Fail(currentTest, "Secret Cache was not used properly")
				return nil, nil
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentTest = t

			tel := nooptelemetry.GetCompatComponent()

			resolver := newEnabledSecretResolver(tel)
			resolver.backendCommand = "some_command"
			if tc.secretCache != nil {
				resolver.cache = tc.secretCache
			}
			resolver.fetchHookFunc = tc.secretFetchCB
			scrubbedKey := []string{}
			resolver.scrubHookFunc = func(k []string) { scrubbedKey = append(scrubbedKey, k[0]) }

			newConf, err := resolver.Resolve(tc.testConf, "test", "", "", true)
			require.NoError(t, err)

			assert.Equal(t, tc.resolvedConf, string(newConf))
			assert.Equal(t, tc.expectedSecretOrigin, resolver.origin)
			assert.Equal(t, tc.expectedScrubbedKey, scrubbedKey)
		})
	}
}

func TestResolveNestedWithSubscribe(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"
	resolver.cache = map[string]string{"pass3": "password3"}

	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return map[string]string{
			"pass1": "password1",
			"pass2": "password2",
		}, nil
	}

	topLevelResolved := 0
	secondLevelResolved := 0
	thirdLevelResolved := 0
	resolver.SubscribeToChanges(func(_, _ string, path []string, _, newValue any) {
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
	_, err := resolver.Resolve(testConfNestedMultiple, "test", "", "", true)

	require.NoError(t, err)
	assert.Equal(t, 1, topLevelResolved, "'top_level' secret was not resolved or resolved multiple times")
	assert.Equal(t, 1, secondLevelResolved, "'second_level' secret was not resolved or resolved multiple times")
	assert.Equal(t, 1, thirdLevelResolved, "'third_level' secret was not resolved or resolved multiple times")

	assert.Equal(t, testConfNestedOriginMultiple, resolver.origin)
}

func TestResolveCached(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"
	resolver.cache = map[string]string{"pass1": "password1"}

	fetchHappened := 0
	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		fetchHappened++
		return map[string]string{
			"pass1": "password1",
		}, nil
	}

	totalResolved := []string{}
	resolver.SubscribeToChanges(func(handle, _ string, _ []string, _, _ any) {
		totalResolved = append(totalResolved, handle)
	})
	_, err := resolver.Resolve(testConfNested, "test", "", "", true)

	// Resolve doesn't need to fetch because value is cached, but subscription is still called
	require.NoError(t, err)
	assert.Equal(t, fetchHappened, 0)
	assert.Equal(t, totalResolved, []string{"pass1"})
}

func TestResolveThenRefresh(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"
	resolver.cache = map[string]string{}

	// subscribe to updates, collect list of keys that get resolved
	keysResolved := []string{}
	oldValues := []string{}
	newValues := []string{}
	resolver.SubscribeToChanges(func(_, _ string, path []string, oldValue, newValue any) {
		keysResolved = append(keysResolved, strings.Join(path, "/"))
		oldValues = append(oldValues, fmt.Sprintf("%s", oldValue))
		newValues = append(newValues, fmt.Sprintf("%s", newValue))
	})

	// initial 3 values for these passwords
	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return map[string]string{
			"pass1": "password1",
			"pass2": "password2",
			"pass3": "password3",
		}, nil
	}

	// resolve the secrets the first time
	_, err := resolver.Resolve(testConfNestedMultiple, "test", "", "", true)
	require.NoError(t, err)
	slices.Sort(keysResolved)
	assert.Equal(t, testConfNestedOriginMultiple, resolver.origin)
	assert.Equal(t, []string{"some/encoded/third_level", "some/second_level", "top_level"}, keysResolved)

	// change the secret value of the handle 'pass2'
	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return map[string]string{
			"pass1": "password1",
			"pass2": "update-second",
			"pass3": "password3",
		}, nil
	}

	// refresh the secrets and only collect newly updated keys
	keysResolved = []string{}
	output, err := resolver.Refresh(true)
	require.NoError(t, err)
	assert.Equal(t, testConfNestedOriginMultiple, resolver.origin)
	assert.Equal(t, []string{"some/second_level"}, keysResolved)
	assert.Contains(t, output, "'pass2'")
	assert.Contains(t, output, "'some/second_level'")
	assert.NotContains(t, output, "update-second")

	// change the secret values of the other two handles
	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return map[string]string{
			"pass1": "update-first",
			"pass2": "update-second",
			"pass3": "update-third",
		}, nil
	}

	// refresh one last time and only those two handles have updated keys
	keysResolved = []string{}
	_, err = resolver.Refresh(true)
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
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"
	resolver.cache = map[string]string{"handle": "value"}
	resolver.origin = handleToContext{
		"handle": []secretContext{
			{
				origin: "datadog.yaml",
				path:   []string{"another", "config", "setting"},
			},
		},
	}

	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return map[string]string{
			"handle": "second_value",
		}, nil
	}
	changes := []string{}
	resolver.SubscribeToChanges(func(_, _ string, _ []string, _, newValue any) {
		changes = append(changes, fmt.Sprintf("%s", newValue))
	})

	originalAllowlistPaths := allowListPaths
	defer func() { allowListPaths = originalAllowlistPaths }()

	// only allow api_key config setting to change
	allowListPaths = []string{"api_key"}

	// Refresh means nothing changes because allowlist doesn't allow it
	_, err := resolver.Refresh(true)
	require.NoError(t, err)
	assert.Equal(t, changes, []string{})

	// now allow the config setting under scrutiny to change
	allowListPaths = []string{"setting"}

	// Refresh sees the change to the handle
	_, err = resolver.Refresh(true)
	require.NoError(t, err)
	assert.Equal(t, changes, []string{"second_value"})
}

// Check that the allowListOrigin works well
func TestRefreshAllowlistFromContainer(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"
	resolver.cache = map[string]string{
		"handle1": "init_value1",
		"handle2": "init_value2",
	}
	resolver.origin = handleToContext{
		"handle1": []secretContext{
			{
				origin: "datadog.yaml",
				path:   []string{"another", "config", "setting"},
			},
			{
				origin: "datadog.yaml",
				path:   []string{"something", "additional_endpoints"},
			},
			{
				origin: "postgres:1234",
				path:   []string{"db_password"},
			},
		},
		"handle2": []secretContext{
			{
				origin: "postgres:1234",
				path:   []string{"db_password_2"},
			},
		},
	}

	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return map[string]string{
			"handle1": "updated_value1",
			"handle2": "updated_value2",
		}, nil
	}
	changes := []string{}
	resolver.SubscribeToChanges(func(handle, origin string, path []string, _, _ any) {
		changes = append(changes, fmt.Sprintf("%s/%s/%v", handle, origin, path))
	})

	// Refresh means nothing changes because allowlist doesn't allow it
	_, err := resolver.Refresh(true)
	require.NoError(t, err)
	slices.Sort(changes)
	assert.Equal(t, changes, []string{
		"handle1/datadog.yaml/[something additional_endpoints]",
		"handle1/postgres:1234/[db_password]",
		"handle2/postgres:1234/[db_password_2]",
	})
}

// test that only setting paths that match the allowlist will get notifications
// about changed secret values from a Refresh
func TestRefreshAllowlistAppliesToEachSettingPath(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"

	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return map[string]string{
			"pass1": "password1",
		}, nil
	}

	// test configuration resolves, the secret appears at two setting paths
	resolved, err := resolver.Resolve(testMultiUsageConf, "datadog.yaml", "", "", true)
	require.NoError(t, err)
	require.Equal(t, testMultiUsageConfResolved, string(resolved))

	// set the allowlist so that only 1 of the settings matches, the 2nd does not
	originalAllowlistPaths := allowListPaths
	allowListPaths = []string{"instances"}
	defer func() { allowListPaths = originalAllowlistPaths }()

	// subscribe to changes made during Refresh, keep track of updated setting paths
	changedPaths := []string{}
	resolver.SubscribeToChanges(func(_, _ string, path []string, _, _ any) {
		changedPaths = append(changedPaths, strings.Join(path, "/"))
	})

	// the secret has a new value, will be picked up by next Refresh
	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return map[string]string{
			"pass1": "second_password",
		}, nil
	}

	// only 1 setting path got updated
	_, err = resolver.Refresh(true)
	require.NoError(t, err)
	assert.Equal(t, changedPaths, []string{"instances/0/password"})
}

// test that adding to the audit file stops working when the file gets too large
func TestRefreshAddsToAuditFile(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "")
	assert.NoError(t, err)

	originalAllowlistPaths := allowListPaths
	allowListPaths = []string{"setting"}
	defer func() { allowListPaths = originalAllowlistPaths }()

	tel := nooptelemetry.GetCompatComponent()
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

	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return map[string]string{
			"handle": "second_value",
		}, nil
	}

	// Refresh the secrets, which will add to the audit file
	_, err = resolver.Refresh(true)
	require.NoError(t, err)
	assert.Equal(t, auditFileNumRows(tmpfile.Name()), 1)

	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return map[string]string{
			"handle": "third_value",
		}, nil
	}

	// Refresh secrets again, which will add another row the audit file
	_, err = resolver.Refresh(true)
	require.NoError(t, err)
	assert.Equal(t, auditFileNumRows(tmpfile.Name()), 2)

	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return map[string]string{
			"handle": "fourth_value",
		}, nil
	}
}

func TestRefreshModes(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"
	resolver.cache = map[string]string{"api_key": "test_key"}
	resolver.origin = handleToContext{
		"api_key": []secretContext{{origin: "test", path: []string{"api_key"}}},
	}

	var calls atomic.Int32
	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		calls.Add(1)
		return map[string]string{"api_key": "test_value"}, nil
	}

	t.Run("updateNow=true refreshes synchronously", func(t *testing.T) {
		calls.Store(0)
		result, err := resolver.Refresh(true)
		require.NoError(t, err)
		assert.NotEmpty(t, result)
		assert.Equal(t, int32(1), calls.Load())
	})

	t.Run("throttling within interval", func(t *testing.T) {
		resolver.apiKeyFailureRefreshInterval = 100 * time.Millisecond
		resolver.lastThrottledRefresh = time.Time{}
		resolver.startRefreshRoutine(nil)

		calls.Store(0)

		// 1 refresh passes, 2 get dropped
		resolver.Refresh(false)
		resolver.Refresh(false)
		resolver.Refresh(false)
		time.Sleep(50 * time.Millisecond)

		assert.Equal(t, int32(1), calls.Load(), "only first refresh should process")

		// after interval, next should succeed
		time.Sleep(100 * time.Millisecond)
		resolver.Refresh(false)
		time.Sleep(50 * time.Millisecond)

		assert.Equal(t, int32(2), calls.Load(), "refresh after interval should process")
	})

	t.Run("feature disabled drops all refreshes", func(t *testing.T) {
		resolver.apiKeyFailureRefreshInterval = 0

		calls.Store(0)
		resolver.Refresh(false)
		resolver.Refresh(false)
		time.Sleep(50 * time.Millisecond)

		assert.Equal(t, int32(0), calls.Load(), "no refreshes when disabled")
	})
}

func TestStartRefreshRoutineWithScatter(t *testing.T) {
	testCases := []struct {
		name                   string
		scatter                bool
		expectedSubsequentTick time.Duration
		r                      *rand.Rand
	}{
		{
			name:                   "Without scatter",
			scatter:                false,
			expectedSubsequentTick: 10 * time.Second,
		},
		{
			name:                   "With scatter",
			scatter:                true,
			expectedSubsequentTick: 10 * time.Second,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			newClock = func() clock.Clock { return clock.NewMock() }
			t.Cleanup(func() {
				newClock = clock.New
			})
			tel := nooptelemetry.GetCompatComponent()

			resolver := newEnabledSecretResolver(tel)
			mockClock := resolver.clk.(*clock.Mock)

			resolver.refreshInterval = 10 * time.Second
			resolver.refreshIntervalScatter = tc.scatter

			if tc.scatter {
				// Seed the random number generator to make the test deterministic
				tc.r = rand.New(rand.NewSource(12345))
			}

			resolver.cache = map[string]string{
				"test-handle": "initial-value",
			}
			resolver.origin = map[string][]secretContext{
				"test-handle": {
					{
						origin: "test-origin",
						path:   []string{"test-path"},
					},
				},
			}

			refreshCalls := 0
			refreshCalledChan := make(chan struct{}, 3)

			resolver.fetchHookFunc = func(_ []string) (map[string]string, error) {
				refreshCalls++
				refreshCalledChan <- struct{}{}

				return map[string]string{
					"test-handle": fmt.Sprintf("updated-value-%d", refreshCalls),
				}, nil
			}
			resolver.startRefreshRoutine(tc.r)

			changeDetected := make(chan struct{}, 3)
			resolver.SubscribeToChanges(func(_, _ string, _ []string, _, _ any) {
				changeDetected <- struct{}{}
			})

			if tc.scatter {
				// The set random seed has a the scatterDuration is 6.477027098s
				mockClock.Add(7 * time.Second)

				select {
				case <-refreshCalledChan:
				case <-time.After(1 * time.Second):
					t.Fatal("First refresh didn't occur even after full interval")
				}
			} else {
				// Without scatter, the first tick should be at the full refresh interval
				mockClock.Add(resolver.refreshInterval)

				select {
				case <-refreshCalledChan:
				case <-time.After(1 * time.Second):
					t.Fatal("First refresh didn't occur at expected time")
				}
			}

			// Now test that subsequent ticks use the full refresh interval regardless of scatter setting
			mockClock.Add(tc.expectedSubsequentTick)

			select {
			case <-refreshCalledChan:
			case <-time.After(1 * time.Second):
				t.Fatal("Second refresh didn't occur at expected time")
			}

			mockClock.Add(tc.expectedSubsequentTick)

			select {
			case <-refreshCalledChan:
			case <-time.After(1 * time.Second):
				t.Fatal("Third refresh didn't occur at expected time")
			}

			if refreshCalls != 3 {
				t.Errorf("Expected 3 refresh calls, got %d", refreshCalls)
			}
		})
	}
}

type alwaysZeroSource struct{}

func (s *alwaysZeroSource) Int63() int64 {
	return 0
}

func (s *alwaysZeroSource) Seed(int64) {}

func TestScatterWithSmallRandomValue(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)

	resolver.refreshInterval = 1 * time.Second
	resolver.refreshIntervalScatter = true
	resolver.fetchHookFunc = func(_ []string) (map[string]string, error) {
		return map[string]string{
			"test-handle": "updated-value",
		}, nil
	}

	// NOTE: clock and ticker are not mocked, as the mock ticker doesn't fail on a
	// zero parameter the way a real ticker does
	r := rand.New(&alwaysZeroSource{})
	ticker := resolver.setupRefreshInterval(r)
	require.NotNil(t, ticker)
	require.True(t, resolver.scatterDuration > 0)

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

func TestBackendTypeWithValidVaultConfig(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	r := newEnabledSecretResolver(tel)

	r.backendType = "hashicorp.vault"
	r.backendConfig = map[string]interface{}{
		"vault_address": "http://127.0.0.1:8200",
		"secret_path":   "/Datadog/Production",
		"vault_session": map[string]interface{}{
			"vault_auth_type": "aws",
			"vault_aws_role":  "rahul_role",
			"aws_region":      "us-east-1",
		},
	}

	r.fetchHookFunc = func([]string) (map[string]string, error) {
		return map[string]string{
			"api_key":     "datadog-api-key-123",
			"app_key":     "datadog-app-key-456",
			"db_password": "secure-db-password",
		}, nil
	}

	r.Configure(secrets.ConfigParams{Type: r.backendType, Config: r.backendConfig})

	assert.Equal(t, "hashicorp.vault", r.backendType)
	assert.Equal(t, "http://127.0.0.1:8200", r.backendConfig["vault_address"])
	assert.Equal(t, "/Datadog/Production", r.backendConfig["secret_path"])

	vaultSession, ok := r.backendConfig["vault_session"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "aws", vaultSession["vault_auth_type"])
	assert.Equal(t, "rahul_role", vaultSession["vault_aws_role"])
	assert.Equal(t, "us-east-1", vaultSession["aws_region"])
}

func TestSecretFiltering(t *testing.T) {
	type testCase struct {
		name         string
		params       secrets.ConfigParams
		expectedConf string
	}

	tests := []testCase{
		{
			name: "ScopeIntegrationToNamespace",
			params: secrets.ConfigParams{
				ScopeIntegrationToNamespace: true,
			},
			expectedConf: `instances:
- some_obj:
  - value1
  - value2
  - ENC[k8s_secret@default/sec1/key1]
`,
		},
		{
			name: "AllowedNamespace",
			params: secrets.ConfigParams{
				AllowedNamespace: []string{"namespace1", "namespace2"},
			},
			expectedConf: `instances:
- some_obj:
  - value1
  - value2
  - ENC[k8s_secret@default/sec1/key1]
`,
		},
		{
			name: "ImageToHandle",
			params: secrets.ConfigParams{
				ImageToHandle: map[string][]string{
					"image1": {"non_k8s_value", "k8s_secret@default/sec1/key1"},
					"image2": {"sev1"},
				},
			},
			expectedConf: `instances:
- some_obj:
  - value1
  - ENC[k8s_secret@namespace1/sec1/key1]
  - value3
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.params.Command = "some_command"
			resolver := newResolver(t, test.params)
			resolver.fetchHookFunc = func([]string) (map[string]string, error) {
				return map[string]string{
					"non_k8s_value":                   "value1",
					"k8s_secret@namespace1/sec1/key1": "value2",
					"k8s_secret@default/sec1/key1":    "value3",
				}, nil
			}

			var resolvedConf []byte
			var err error

			// Some source might have a namespace but not a image name or the opposite
			if test.params.ImageToHandle == nil {
				resolvedConf, err = resolver.Resolve(testSecretFiltering, "container:123", "", "namespace1", true)
			} else {
				resolvedConf, err = resolver.Resolve(testSecretFiltering, "container:123", "image1", "", true)
			}
			assert.NoError(t, err)
			assert.Equal(t, test.expectedConf, string(resolvedConf))

			// This test verify that any secrets from non-container sources can still be resolved. Non-container
			// configuration are datadog.yaml, system-probe.yaml, integrations from files, ...
			expectedConf := `instances:
- some_obj:
  - value1
  - value2
  - value3
`
			resolvedConf, err = resolver.Resolve(testSecretFiltering, "datadog.yaml", "", "", true)
			assert.NoError(t, err)
			assert.Equal(t, expectedConf, string(resolvedConf))
		})
	}
}

func TestRemoveOrigin(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()

	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"
	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return map[string]string{
			"pass1": "password1",
			"pass2": "password2",
			"pass3": "password3",
		}, nil
	}

	conf := []byte("test: [\"ENC[pass1]\", \"ENC[pass2]\"]")
	conf2 := []byte("test2: ENC[pass3]")

	_, err := resolver.Resolve(conf, "origin1", "", "", true)
	require.NoError(t, err)
	_, err = resolver.Resolve(conf, "origin2", "", "", true)
	require.NoError(t, err)
	_, err = resolver.Resolve(conf2, "origin3", "", "", true)
	require.NoError(t, err)

	assert.Equal(t, handleToContext{
		"pass1": []secretContext{
			{origin: "origin1", path: []string{"test", "0"}},
			{origin: "origin2", path: []string{"test", "0"}},
		},
		"pass2": []secretContext{
			{origin: "origin1", path: []string{"test", "1"}},
			{origin: "origin2", path: []string{"test", "1"}},
		},
		"pass3": []secretContext{
			{origin: "origin3", path: []string{"test2"}},
		},
	}, resolver.origin)

	resolver.RemoveOrigin("origin2")

	assert.Equal(t, handleToContext{
		"pass1": []secretContext{
			{origin: "origin1", path: []string{"test", "0"}},
		},
		"pass2": []secretContext{
			{origin: "origin1", path: []string{"test", "1"}},
		},
		"pass3": []secretContext{
			{origin: "origin3", path: []string{"test2"}},
		},
	}, resolver.origin)

	resolver.RemoveOrigin("unknown")
	assert.Equal(t, handleToContext{
		"pass1": []secretContext{
			{origin: "origin1", path: []string{"test", "0"}},
		},
		"pass2": []secretContext{
			{origin: "origin1", path: []string{"test", "1"}},
		},
		"pass3": []secretContext{
			{origin: "origin3", path: []string{"test2"}},
		},
	}, resolver.origin)

	resolver.RemoveOrigin("origin1")
	assert.Equal(t, handleToContext{
		"pass3": []secretContext{
			{origin: "origin3", path: []string{"test2"}},
		},
	}, resolver.origin)

	resolver.RemoveOrigin("origin3")
	assert.Equal(t, handleToContext{}, resolver.origin)
}

func TestRefreshOutput(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	password := "password1"

	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"
	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return map[string]string{
			"pass1": password,
		}, nil
	}

	_, err := resolver.Resolve(testSimpleConf, "origin1", "", "", true)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		resolver.SubscribeToChanges(func(_, _ string, _ []string, _, _ any) {})
	}

	password = "password2"

	res, err := resolver.Refresh(true)
	require.NoError(t, err)
	res = strings.ReplaceAll(res, "\r", "") // templates use OS line breaks, removes \r line breaks from windows
	assert.Equal(t, "=== Secret stats ===\nNumber of secrets reloaded: 1\nSecrets handle reloaded:\n\n- 'pass1':\n\tused in 'origin1' configuration in entry 'secret_backend_arguments/0'\n", res)
}

func TestResolveNoNotify(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()

	password := "password1"

	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"
	resolver.fetchHookFunc = func([]string) (map[string]string, error) {
		return map[string]string{
			"pass1": password,
		}, nil
	}
	resolver.SubscribeToChanges(func(_, _ string, _ []string, _, _ any) {
		assert.Fail(t, "test should not have send notifications")
	})

	_, err := resolver.Resolve(testSimpleConf, "origin1", "", "", false)
	require.NoError(t, err)
}
