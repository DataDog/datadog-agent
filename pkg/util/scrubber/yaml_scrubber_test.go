// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/yaml.v3"
)

func TestScrubDataObj(t *testing.T) {
	testCases := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name: "Scrub sensitive info from map",
			input: map[string]interface{}{
				"password": "password123",
				"username": "user1",
			},
			expected: map[string]interface{}{
				"password": "********",
				"username": "user1",
			},
		},
		{
			name: "SNMPConfig",
			input: map[string]interface{}{
				"community_string": "password123",
				"authKey":          "password",
				"authkey":          "password",
				"privKey":          "password",
				"privkey":          "password",
			},
			expected: map[string]interface{}{
				"community_string": "********",
				"authKey":          "********",
				"authkey":          "********",
				"privKey":          "********",
				"privkey":          "********",
			},
		},
		{
			name: "Scrub sensitive info from nested map",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"password": "password123",
					"email":    "user@example.com",
				},
			},
			expected: map[string]interface{}{
				"user": map[string]interface{}{
					"password": "********",
					"email":    "user@example.com",
				},
			},
		},
		{
			name:     "No sensitive info to scrub",
			input:    "Just a regular string.",
			expected: "Just a regular string.",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ScrubDataObj(&tc.input)
			assert.Equal(t, tc.expected, tc.input)
		})
	}
}

func TestConfigScrubbedValidYaml(t *testing.T) {
	wd, _ := os.Getwd()

	inputConf := filepath.Join(wd, "test", "conf.yaml")
	inputConfData, err := os.ReadFile(inputConf)
	require.NoError(t, err)

	outputConf := filepath.Join(wd, "test", "conf_scrubbed.yaml")
	outputConfData, err := os.ReadFile(outputConf)
	require.NoError(t, err)

	cleaned, err := ScrubBytes([]byte(inputConfData))
	require.NoError(t, err)

	// First test that the a scrubbed yaml is still a valid yaml
	var out interface{}
	err = yaml.Unmarshal(cleaned, &out)
	assert.NoError(t, err, "Could not load YAML configuration after being scrubbed")

	// We replace windows line break by linux so the tests pass on every OS
	trimmedOutput := strings.TrimSpace(strings.ReplaceAll(string(outputConfData), "\r\n", "\n"))
	trimmedCleaned := strings.TrimSpace(strings.ReplaceAll(string(cleaned), "\r\n", "\n"))

	assert.Equal(t, trimmedOutput, trimmedCleaned)
}

func TestConfigScrubbedYaml(t *testing.T) {
	wd, _ := os.Getwd()

	inputConf := filepath.Join(wd, "test", "conf_multiline.yaml")
	inputConfData, err := os.ReadFile(inputConf)
	require.NoError(t, err)

	outputConf := filepath.Join(wd, "test", "conf_multiline_scrubbed.yaml")
	outputConfData, err := os.ReadFile(outputConf)
	require.NoError(t, err)

	cleaned, err := ScrubYaml([]byte(inputConfData))
	require.NoError(t, err)

	// First test that the a scrubbed yaml is still a valid yaml
	var out interface{}
	err = yaml.Unmarshal(cleaned, &out)
	assert.NoError(t, err, "Could not load YAML configuration after being scrubbed")

	// We replace windows line break by linux so the tests pass on every OS
	trimmedOutput := strings.TrimSpace(strings.ReplaceAll(string(outputConfData), "\r\n", "\n"))
	trimmedCleaned := strings.TrimSpace(strings.ReplaceAll(string(cleaned), "\r\n", "\n"))

	assert.Equal(t, trimmedOutput, trimmedCleaned)
}

func TestEmptyYaml(t *testing.T) {
	cleaned, err := ScrubYaml(nil)
	require.NoError(t, err)
	assert.Equal(t, "", string(cleaned))

	cleaned, err = ScrubYaml([]byte(""))
	require.NoError(t, err)
	assert.Equal(t, "", string(cleaned))
}

func TestEmptyYamlString(t *testing.T) {
	cleaned, err := ScrubYamlString("")
	require.NoError(t, err)
	assert.Equal(t, "", string(cleaned))
}

func TestAddStrippedKeysExceptions(t *testing.T) {
	t.Run("single key", func(t *testing.T) {
		contents := `api_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'`

		AddStrippedKeys([]string{"api_key"})

		scrubbed, err := ScrubYamlString(contents)
		require.Nil(t, err)
		require.YAMLEq(t, `api_key: '***************************aaaaa'`, scrubbed)
	})

	t.Run("multiple keys", func(t *testing.T) {
		contents := `api_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
some_other_key: 'bbbb'
app_key: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaacccc'
yet_another_key: 'dddd'`

		keys := []string{"api_key", "some_other_key", "app_key"}
		AddStrippedKeys(keys)

		// check that AddStrippedKeys didn't modify the parameter slice
		assert.Equal(t, []string{"api_key", "some_other_key", "app_key"}, keys)

		scrubbed, err := ScrubYamlString(contents)
		require.Nil(t, err)
		expected := `api_key: '***************************aaaaa'
some_other_key: '********'
app_key: '***********************************acccc'
yet_another_key: 'dddd'`
		require.YAMLEq(t, expected, scrubbed)
	})
}
