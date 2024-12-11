// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configcheck

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type baseConfigCheckSuite struct {
	e2e.BaseSuite[environments.Host]
}

type CheckConfigOutput struct {
	CheckName  string
	Filepath   string
	InstanceID string
	Settings   string
}

func MatchCheckToTemplate(checkname, input string) (*CheckConfigOutput, error) {
	regexTemplate := fmt.Sprintf("=== %s check ===\n", checkname) +
		"Configuration provider: file\n" +
		"Configuration source: (?P<filepath>.*)\n" +
		"Config for instance ID: (?P<instance>.*)\n" +
		"(?P<settings>(?m:^[^=].*\n)+)" + // non-capturing group to get all settings
		"==="
	re := regexp.MustCompile(regexTemplate)
	matches := re.FindStringSubmatch(input)

	// without a match, SubexpIndex lookups panic with range errors
	if len(matches) == 0 {
		return nil, fmt.Errorf("regexp: no matches for %s check", checkname)
	}

	filepathIndex := re.SubexpIndex("filepath")
	instanceIndex := re.SubexpIndex("instance")
	settingsIndex := re.SubexpIndex("settings")

	return &CheckConfigOutput{
		CheckName:  checkname,
		Filepath:   matches[filepathIndex],
		InstanceID: matches[instanceIndex],
		Settings:   matches[settingsIndex],
	}, nil
}

func TestMatchToTemplateHelper(t *testing.T) {
	sampleCheck := `=== uptime check ===
Configuration provider: file
Configuration source: file:/etc/datadog-agent/conf.d/uptime.d/conf.yaml.default
Config for instance ID: uptime:c72f390abdefdf1a
key: value
path: http://example.com/foo
~
===

=== ntp check ===
Configuration provider: file
Configuration source: file:/etc/datadog-agent/conf.d/npt.d/conf.yaml.default
Config for instance ID: npt:c72f390abdefdf1a
{}
~
===

=== cpu check ===
Configuration provider: file
Configuration source: file:/etc/datadog-agent/conf.d/cpu.d/conf.yaml.default
Config for instance ID: cpu:e331d61ed1323219
{}
~
===`

	t.Run("uptime", func(t *testing.T) {
		result, err := MatchCheckToTemplate("uptime", sampleCheck)
		require.NoError(t, err)

		assert.Contains(t, result.CheckName, "uptime")
		assert.Contains(t, result.Filepath, "file:/etc/datadog-agent/conf.d/uptime.d/conf.yaml.default")
		assert.Contains(t, result.InstanceID, "uptime:c72f390abdefdf1a")
		assert.Contains(t, result.Settings, "key: value")
		assert.Contains(t, result.Settings, "path: http://example.com/foo")
		assert.NotContains(t, result.Settings, "{}")
	})

	t.Run("cpu", func(t *testing.T) {
		result, err := MatchCheckToTemplate("cpu", sampleCheck)
		require.NoError(t, err)

		assert.Contains(t, result.CheckName, "cpu")
		assert.Contains(t, result.Filepath, "file:/etc/datadog-agent/conf.d/cpu.d/conf.yaml.default")
		assert.Contains(t, result.InstanceID, "cpu:e331d61ed1323219")
		assert.Contains(t, result.Settings, "{}")
	})
}

func VerifyDefaultInstalledCheck(t *testing.T, output string, testChecks []CheckConfigOutput) {
	assert.NotContains(t, output, "=== Configuration errors ===")

	for _, testCheck := range testChecks {
		t.Run(fmt.Sprintf("default - %s test", testCheck.CheckName), func(t *testing.T) {
			result, err := MatchCheckToTemplate(testCheck.CheckName, output)
			require.NoError(t, err)
			assert.Contains(t, result.Filepath, testCheck.Filepath)
			assert.Contains(t, result.InstanceID, testCheck.InstanceID)
			assert.Contains(t, result.Settings, testCheck.Settings)
		})
	}
}
