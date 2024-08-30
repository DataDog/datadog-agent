// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config/legacy"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestBasicMinCollectionIntervalRelocation(t *testing.T) {
	input := `
init_config:
   min_collection_interval: 30
instances: [{}, {}]`
	output := `
init_config: {}
instances:
  - min_collection_interval: 30
  - min_collection_interval: 30`
	assertRelocation(t, input, output)
}

func TestEmptyInstances(t *testing.T) {
	input := `
init_config:
   min_collection_interval: 30
instances:`
	output := `
init_config: {}
instances:`
	assertRelocation(t, input, output)
}

func TestEmptyYaml(t *testing.T) {
	assertRelocation(t, "", "")
}
func TestUntouchedYaml(t *testing.T) {
	input := `
instances:
  - host: localhost
    port: 7199
    cassandra_aliasing: true
init_config:
  is_jmx: true
  collect_default_metrics: true`
	assertRelocation(t, input, input)
}

func assertRelocation(t *testing.T, input, expectedOutput string) {
	output, _ := relocateMinCollectionInterval([]byte(input))
	expectedYamlOuput := make(map[interface{}]interface{})
	yamlOutput := make(map[interface{}]interface{})
	yaml.Unmarshal(output, &yamlOutput)
	yaml.Unmarshal([]byte(expectedOutput), &expectedYamlOuput)
	assert.Equal(t, yamlOutput, expectedYamlOuput)
}

func TestImport(t *testing.T) {
	integrations := []string{"cassandra", "kubelet", "mysql"}
	RunImport(t, integrations)
}

func RunImport(t *testing.T, integrations []string) {
	mock.New(t)
	a6ConfDir := t.TempDir()
	a5ConfDir := path.Join(".", "tests", "a5_conf")
	a6RefConfDir := path.Join(".", "tests", "a6_conf")

	err := ImportConfig(a5ConfDir, a6ConfDir, false)
	require.NoError(t, err, "ImportConfig failed")
	assert.FileExists(t, path.Join(a6ConfDir, "datadog.yaml"), "datadog.yaml is missing")
	validateSelectedParameters(t, path.Join(a6ConfDir, "datadog.yaml"), path.Join(a5ConfDir, "datadog.conf"))

	// Check integrations import are correct
	for _, i := range integrations {
		assert.FileExists(t, path.Join(a6ConfDir, "conf.d", i+".d", "conf.yaml"), i+".d/conf.yaml is missing")
		assertYAMLEquality(t,
			path.Join(a6RefConfDir, "conf.d", i+".d", "conf.yaml"),
			path.Join(a6ConfDir, "conf.d", i+".d", "conf.yaml"))
	}

	// Ensure we don't overwrite if we are not forced to
	err = ImportConfig(path.Join(".", "tests", "a5_conf"), a6ConfDir, false)
	require.Error(t, err, "ImportConfig should have failed")

	// Ensure we backup file if we force overwriting
	err = ImportConfig(path.Join(".", "tests", "a5_conf"), a6ConfDir, true)
	require.NoError(t, err, "ImportConfig failed")
	for _, i := range integrations {
		assert.FileExists(t, path.Join(a6ConfDir, "conf.d", i+".d", "conf.yaml.bak"), i+".d/conf.yaml.bak is missing")
	}
}

type paramMatcher struct {
	oldKey    string
	newKey    string
	translate func(interface{}) interface{}
}

func validateSelectedParameters(t *testing.T, migratedConfigFile, oldConfigFile string) {
	migratedBytes, err := os.ReadFile(migratedConfigFile)
	require.NoError(t, err, "Failed to read"+migratedConfigFile)
	migratedConf := make(map[string]interface{})
	yaml.Unmarshal(migratedBytes, migratedConf)

	oldConfig, err := legacy.GetAgentConfig(oldConfigFile)
	require.NoError(t, err, "Failed to read"+oldConfigFile)

	// Top level parameters
	params := []paramMatcher{
		{"dd_url", "dd_url", func(v interface{}) interface{} { return v }},
		{"api_key", "api_key", func(v interface{}) interface{} { return v }},
		{"skip_ssl_validation", "skip_ssl_validation", toBool},
		{"hostname", "hostname", func(v interface{}) interface{} { return v }},
		{"enable_gohai", "enable_gohai", toBool},
		{"forwarder_timeout", "forwarder_timeout", toInt},
		{"default_integration_http_timeout", "default_integration_http_timeout", toInt},
		{"collect_ec2_tags", "collect_ec2_tags", toBool},
		{"bind_host", "bind_host", func(v interface{}) interface{} { return v }},
		{"sd_template_dir", "autoconf_template_dir", func(v interface{}) interface{} { return v }},
		{"use_dogstatsd", "use_dogstatsd", toBool},
		{"dogstatsd_port", "dogstatsd_port", toInt},
		{"log_level", "log_level", func(v interface{}) interface{} { return v }},
	}

	for _, p := range params {
		assert.Equal(t, p.translate(oldConfig[p.oldKey]), migratedConf[p.newKey], "wrong conversion for key: "+p.newKey)
	}

	// proxy settings
	oldProxies, err := legacy.BuildProxySettings(oldConfig)
	require.NoError(t, err, "Failed to read old proxy settings")
	migratedProxies := migratedConf["proxy"].(map[interface{}]interface{})
	assert.Equal(t, oldProxies["https"], migratedProxies["https"])
	assert.Equal(t, oldProxies["http"], migratedProxies["http"])

	// Tags
	oldTags := strings.Split(oldConfig["tags"], ",")
	for i, tag := range oldTags {
		oldTags[i] = strings.TrimSpace(tag)
	}
	assert.ElementsMatch(t, oldTags, migratedConf["tags"].([]interface{}))

	// Some second level parameters
	migratedProcessConfig := migratedConf["process_config"].(map[interface{}]interface{})
	processConfigProcessCollection := migratedProcessConfig["process_collection"].(map[interface{}]interface{})
	assert.Equal(t, oldConfig["process_agent_enabled"], strconv.FormatBool(processConfigProcessCollection["enabled"].(bool)))

	migratedApmConfig := migratedConf["apm_config"].(map[interface{}]interface{})
	assert.Equal(t, toBool(oldConfig["apm_enabled"]), migratedApmConfig["enabled"])
}

func toBool(val interface{}) interface{} {
	v := strings.ToLower(val.(string))
	return v == "true" || v == "yes" || v == "1" || v == "on"
}

func toInt(val interface{}) interface{} {
	v, _ := strconv.Atoi(val.(string))
	return v
}

func assertYAMLEquality(t *testing.T, f1, f2 string) {
	f1Bytes, err := os.ReadFile(f1)
	require.NoError(t, err, "Failed to read "+f1)
	migratedContent := make(map[string]interface{})
	yaml.Unmarshal(f1Bytes, migratedContent)

	f2Bytes, err := os.ReadFile(f2)
	require.NoError(t, err, "Failed to read "+f2)
	expectedContent := make(map[string]interface{})
	yaml.Unmarshal(f2Bytes, expectedContent)

	assert.Equal(t, expectedContent, migratedContent)
}
