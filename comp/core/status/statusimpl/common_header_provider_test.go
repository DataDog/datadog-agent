// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestCommonHeaderProviderIndex(t *testing.T) {
	config := config.NewMock(t)

	provider := newCommonHeaderProvider(agentParams, config)

	assert.Equal(t, 0, provider.Index())
}

func TestCommonHeaderProviderJSON(t *testing.T) {
	nowFunc = func() time.Time { return time.Unix(1515151515, 0) }
	startTimeProvider = time.Unix(1515151515, 0)
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")

	defer func() {
		nowFunc = time.Now
		startTimeProvider = pkgconfigsetup.StartTime
		os.Setenv("TZ", originalTZ)
	}()

	config := config.NewMock(t)

	provider := newCommonHeaderProvider(agentParams, config)
	statusData := map[string]interface{}{}
	provider.JSON(false, statusData)

	assert.Equal(t, version.AgentVersion, statusData["version"])
	assert.Equal(t, agentFlavor, statusData["flavor"])
	assert.Equal(t, config.ConfigFileUsed(), statusData["conf_file"])
	assert.Equal(t, pid, statusData["pid"])
	assert.Equal(t, goVersion, statusData["go_version"])
	assert.Equal(t, startTimeProvider.UnixNano(), statusData["agent_start_nano"])
	assert.Equal(t, "n/a", statusData["python_version"])
	assert.Equal(t, arch, statusData["build_arch"])
	assert.Equal(t, nowFunc().UnixNano(), statusData["time_nano"])
	assert.NotEqual(t, "", statusData["title"])
	assert.Equal(t, "not available", statusData["fips_status"])
}

func TestCommonHeaderProviderText(t *testing.T) {
	nowFunc = func() time.Time { return time.Unix(1515151515, 0) }
	startTimeProvider = time.Unix(1515151515, 0)

	defer func() {
		nowFunc = time.Now
		startTimeProvider = pkgconfigsetup.StartTime
	}()

	config := config.NewMock(t)

	provider := newCommonHeaderProvider(agentParams, config)

	buffer := new(bytes.Buffer)
	provider.Text(false, buffer)

	expectedTextOutput := fmt.Sprintf(`  Status date: 2018-01-05 11:25:15 UTC (1515151515000)
  Agent start: 2018-01-05 11:25:15 UTC (1515151515000)
  Pid: %d
  Go Version: %s
  Python Version: n/a
  Build arch: %s
  Agent flavor: %s
  FIPS Mode: not available
  Log Level: info

  Paths
  =====
    Config File: There is no config file
    conf.d: %s
    checks.d: %s
`, pid, goVersion, arch, agentFlavor, config.GetString("confd_path"), config.GetString("additional_checksd"))

	// We replace windows line break by linux so the tests pass on every OS
	expectedResult := strings.Replace(expectedTextOutput, "\r\n", "\n", -1)
	output := strings.Replace(buffer.String(), "\r\n", "\n", -1)

	assert.Equal(t, expectedResult, output)
}

func TestCommonHeaderProviderTime(t *testing.T) {
	// test that the time is updated on each call
	counter := 0
	nowFunc = func() time.Time {
		counter++
		return time.Unix(int64(counter), 0)
	}
	defer func() { nowFunc = time.Now }()

	config := config.NewMock(t)

	provider := newCommonHeaderProvider(agentParams, config)

	data := map[string]interface{}{}
	err := provider.JSON(false, data)
	require.NoError(t, err)
	require.Contains(t, data, "time_nano")
	assert.EqualValues(t, int64(1000000000), data["time_nano"])

	clear(data)
	err = provider.JSON(false, data)
	require.NoError(t, err)
	require.Contains(t, data, "time_nano")
	assert.EqualValues(t, int64(2000000000), data["time_nano"])
}

func assertLogLevel(t *testing.T, provider status.HeaderProvider, expected string) {
	t.Helper()

	data := map[string]interface{}{}
	err := provider.JSON(false, data)
	require.NoError(t, err)

	require.Contains(t, data, "config")
	require.Contains(t, data["config"], "log_level")

	cfg, ok := data["config"].(map[string]string)
	require.True(t, ok)

	require.EqualValues(t, expected, cfg["log_level"])
}

func TestCommonHeaderProviderConfig(t *testing.T) {
	config := config.NewMock(t)
	provider := newCommonHeaderProvider(agentParams, config)

	config.Set("log_level", "info", model.SourceAgentRuntime)
	assertLogLevel(t, provider, "info")

	config.Set("log_level", "warn", model.SourceAgentRuntime)
	assertLogLevel(t, provider, "warn")
}

func TestCommonHeaderProviderHTML(t *testing.T) {
	nowFunc = func() time.Time { return time.Unix(1515151515, 0) }
	startTimeProvider = time.Unix(1515151515, 0)
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")

	defer func() {
		nowFunc = time.Now
		startTimeProvider = pkgconfigsetup.StartTime
		os.Setenv("TZ", originalTZ)
	}()

	config := config.NewMock(t)

	provider := newCommonHeaderProvider(agentParams, config)

	buffer := new(bytes.Buffer)
	provider.HTML(false, buffer)

	// We have to do this strings replacement because html/temaplte escapes the `+` sign
	// https://github.com/golang/go/issues/42506
	result := buffer.String()
	unescapedResult := strings.Replace(result, "&#43;", "+", -1)

	expectedHTMLOutput := fmt.Sprintf(`<div class="stat">
  <span class="stat_title">Agent Info</span>
  <span class="stat_data">
    Version: %s<br>
    Flavor: %s<br>
    PID: %d<br>
    Agent start: 2018-01-05 11:25:15 UTC (1515151515000)<br>
    FIPS Mode: not available<br>
    Log Level: info<br>
    Config File: There is no config file<br>
    Conf.d Path: %s<br>
    Checks.d Path: %s
  </span>
</div>

<div class="stat">
  <span class="stat_title">System Info</span>
  <span class="stat_data">
    System time: 2018-01-05 11:25:15 UTC (1515151515000)
    <br>Go Version: %s
    <br>Python Version: n/a
    <br>Build arch: %s
  </span>
</div>
`, version.AgentVersion, agentFlavor, pid, config.GetString("confd_path"), config.GetString("additional_checksd"), goVersion, arch)

	// We replace windows line break by linux so the tests pass on every OS
	expectedResult := strings.Replace(expectedHTMLOutput, "\r\n", "\n", -1)
	output := strings.Replace(unescapedResult, "\r\n", "\n", -1)

	assert.Equal(t, expectedResult, output)
}
