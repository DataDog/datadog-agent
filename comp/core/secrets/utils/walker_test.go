// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v2"
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
)

func TestWalkerError(t *testing.T) {
	var config interface{}
	err := yaml.Unmarshal(testYamlHash, &config)
	require.NoError(t, err)

	w := Walker{
		Resolver: func([]string, string) (string, error) {
			return "", errors.New("some error")
		},
	}

	err = w.Walk(&config)
	assert.NotNil(t, err)
}

func TestWalkerComplex(t *testing.T) {
	var config interface{}
	err := yaml.Unmarshal(testYamlHash, &config)
	require.NoError(t, err)

	stringsCollected := []string{}
	w := Walker{
		Resolver: func(_ []string, str string) (string, error) {
			stringsCollected = append(stringsCollected, str)
			return str + "_verified", nil
		},
	}
	err = w.Walk(&config)
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
