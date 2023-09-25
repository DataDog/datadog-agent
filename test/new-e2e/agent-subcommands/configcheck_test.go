// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
)

type agentConfigCheckSuite struct {
	e2e.Suite[e2e.AgentEnv]
}

func TestAgentConfigCheckSuite(t *testing.T) {
	e2e.Run(t, &agentConfigCheckSuite{}, e2e.AgentStackDef())
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

func (v *agentConfigCheckSuite) TestMatchToTemplateHelper() {
	sampleCheck := `=== uptime check ===
Configuration provider: file
Configuration source: file:/etc/datadog-agent/conf.d/uptime.d/conf.yaml.default
Config for instance ID: uptime:c72f390abdefdf1a
key: value
path: http://example.com/foo
~
===

=== npt check ===
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

	result, err := MatchCheckToTemplate("uptime", sampleCheck)
	assert.NoError(v.T(), err)

	assert.Contains(v.T(), result.CheckName, "uptime")
	assert.Contains(v.T(), result.Filepath, "file:/etc/datadog-agent/conf.d/uptime.d/conf.yaml.default")
	assert.Contains(v.T(), result.InstanceID, "uptime:c72f390abdefdf1a")
	assert.Contains(v.T(), result.Settings, "key: value")
	assert.Contains(v.T(), result.Settings, "path: http://example.com/foo")
	assert.NotContains(v.T(), result.Settings, "{}")

	result, err = MatchCheckToTemplate("cpu", sampleCheck)
	assert.NoError(v.T(), err)

	assert.Contains(v.T(), result.CheckName, "cpu")
	assert.Contains(v.T(), result.Filepath, "file:/etc/datadog-agent/conf.d/cpu.d/conf.yaml.default")
	assert.Contains(v.T(), result.InstanceID, "cpu:e331d61ed1323219")
	assert.Contains(v.T(), result.Settings, "{}")
}

// cpu, disk, file_handle, io, load, memory, network, ntp, uptime
func (v *agentConfigCheckSuite) TestDefaultInstalledChecks() {
	v.UpdateEnv(e2e.AgentStackDef())

	testChecks := []CheckConfigOutput{
		{
			CheckName:  "cpu",
			Filepath:   "file:/etc/datadog-agent/conf.d/cpu.d/conf.yaml.default",
			InstanceID: "cpu:",
			Settings:   "{}",
		},
		{
			CheckName:  "disk",
			Filepath:   "file:/etc/datadog-agent/conf.d/disk.d/conf.yaml.default",
			InstanceID: "disk:",
			Settings:   "use_mount: false",
		},
		{
			CheckName:  "file_handle",
			Filepath:   "file:/etc/datadog-agent/conf.d/file_handle.d/conf.yaml.default",
			InstanceID: "file_handle:",
			Settings:   "{}",
		},
		{
			CheckName:  "io",
			Filepath:   "file:/etc/datadog-agent/conf.d/io.d/conf.yaml.default",
			InstanceID: "io:",
			Settings:   "{}",
		},
		{
			CheckName:  "load",
			Filepath:   "file:/etc/datadog-agent/conf.d/load.d/conf.yaml.default",
			InstanceID: "load:",
			Settings:   "{}",
		},
		{
			CheckName:  "memory",
			Filepath:   "file:/etc/datadog-agent/conf.d/memory.d/conf.yaml.default",
			InstanceID: "memory:",
			Settings:   "{}",
		},
		{
			CheckName:  "network",
			Filepath:   "file:/etc/datadog-agent/conf.d/network.d/conf.yaml.default",
			InstanceID: "network:",
			Settings:   "{}",
		},
		{
			CheckName:  "ntp",
			Filepath:   "file:/etc/datadog-agent/conf.d/ntp.d/conf.yaml.default",
			InstanceID: "ntp:",
			Settings:   "{}",
		},
		{
			CheckName:  "uptime",
			Filepath:   "file:/etc/datadog-agent/conf.d/uptime.d/conf.yaml.default",
			InstanceID: "uptime:",
			Settings:   "{}",
		},
	}

	output := v.Env().Agent.ConfigCheck()

	assert.NotContains(v.T(), output, "=== Configuration errors ===")

	for _, testCheck := range testChecks {
		v.T().Run(fmt.Sprintf("default - %s test", testCheck.CheckName), func(t *testing.T) {
			result, err := MatchCheckToTemplate(testCheck.CheckName, output)
			assert.NoError(t, err)
			assert.Contains(t, result.Filepath, testCheck.Filepath)
			assert.Contains(t, result.InstanceID, testCheck.InstanceID)
			assert.Contains(t, result.Settings, testCheck.Settings)
		})
	}
}

func (v *agentConfigCheckSuite) TestWithBadConfigCheck() {
	// invalid config because of tabspace
	config := `instances:
	- name: bad yaml formatting via tab
`
	integration := agentparams.WithIntegration("http_check.d", config)
	v.UpdateEnv(e2e.AgentStackDef(e2e.WithAgentParams(integration)))

	output := v.Env().Agent.ConfigCheck()

	assert.Contains(v.T(), output, "http_check: yaml: line 2: found character that cannot start any token")
}

func (v *agentConfigCheckSuite) TestWithAddedIntegrationsCheck() {
	config := `instances:
  - name: My First Service
    url: http://some.url.example.com
`
	integration := agentparams.WithIntegration("http_check.d", config)
	v.UpdateEnv(e2e.AgentStackDef(e2e.WithAgentParams(integration)))

	output := v.Env().Agent.ConfigCheck()

	result, err := MatchCheckToTemplate("http_check", output)
	assert.NoError(v.T(), err)
	assert.Contains(v.T(), result.Filepath, "file:/etc/datadog-agent/conf.d/http_check.d/conf.yaml")
	assert.Contains(v.T(), result.InstanceID, "http_check:")
	assert.Contains(v.T(), result.Settings, "name: My First Service")
	assert.Contains(v.T(), result.Settings, "url: http://some.url.example.com")
}
