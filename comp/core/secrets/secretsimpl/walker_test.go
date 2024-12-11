// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsimpl

import (
	"fmt"
	"sort"
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
