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
	yaml "gopkg.in/yaml.v2"
)

func TestWalkerError(t *testing.T) {
	var config interface{}
	err := yaml.Unmarshal(testYamlHash, &config)
	require.NoError(t, err)

	w := walker{
		resolver: func([]string, string) (string, error) {
			return "", fmt.Errorf("some error")
		},
	}

	err = w.walk(&config)
	assert.NotNil(t, err)
}

func TestWalkerSimple(t *testing.T) {
	var config interface{}
	err := yaml.Unmarshal([]byte("test"), &config)
	require.NoError(t, err)

	stringsCollected := []string{}

	w := walker{
		resolver: func(_ []string, str string) (string, error) {
			stringsCollected = append(stringsCollected, str)
			return str + "_verified", nil
		},
	}
	err = w.walk(&config)
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
	w := walker{
		resolver: func(_ []string, str string) (string, error) {
			stringsCollected = append(stringsCollected, str)
			return str + "_verified", nil
		},
	}
	err = w.walk(&config)
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

func TestWalkerNotify(t *testing.T) {
	yamlConf := []byte(`
slice:
  - "ENC 1"
  - ["ENC test1", test2]
  - 123
hash:
  a: test3
  b: "ENC 2"
  c: 456
  slice:
    - ENC test4
    - test5
`)

	notification := map[string]any{}
	w := walker{
		resolver: func(_ []string, value string) (string, error) {
			if strings.HasPrefix(value, "ENC ") {
				return value[4:] + "_verified", nil
			}
			return value, nil
		},
		notifier: func(yamlPath []string, value any) {
			notification[strings.Join(yamlPath, ".")] = value
		},
	}

	var config interface{}
	err := yaml.Unmarshal(yamlConf, &config)
	require.NoError(t, err)

	err = w.walk(&config)
	require.NoError(t, err)

	// we expect to be notified once for each updated value (especially a single call per slice)
	expected := map[string]any{
		"hash.b":     "2_verified",
		"hash.slice": []any{"test4_verified", "test5"},
		"slice": []any{
			"1_verified",
			[]any{"test1_verified", "test2"},
			123,
		},
	}
	assert.Equal(t, expected, notification)
}
