// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"os"
	"path"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestGetIntegrationConfig(t *testing.T) {
	// file does not exist
	_, _, err := GetIntegrationConfigFromFile("foo", "")
	assert.NotNil(t, err)

	// file contains invalid Yaml
	_, _, err = GetIntegrationConfigFromFile("foo", "tests/invalid.yaml")
	assert.NotNil(t, err)

	// valid yaml but not a valid integration configuration
	config, _, err := GetIntegrationConfigFromFile("foo", "tests/notaconfig.yaml")
	assert.NotNil(t, err)
	assert.Equal(t, len(config.Instances), 0)

	// empty file
	config, _, err = GetIntegrationConfigFromFile("foo", "tests/empty.yaml")
	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), emptyFileError)
	assert.Equal(t, len(config.Instances), 0)

	// valid yaml with a stub integration instance
	config, _, err = GetIntegrationConfigFromFile("foo", "tests/stub.yaml")
	assert.Nil(t, err)
	assert.Equal(t, len(config.Instances), 1)

	// valid yaml, instances array is null
	config, _, err = GetIntegrationConfigFromFile("foo", "tests/null_instances.yaml")
	assert.NotNil(t, err)
	assert.Equal(t, len(config.Instances), 0)

	// valid metric file
	config, _, err = GetIntegrationConfigFromFile("foo", "tests/metrics.yaml")
	assert.Nil(t, err)
	assert.NotNil(t, config.MetricConfig)

	// valid logs-agent file
	config, _, err = GetIntegrationConfigFromFile("foo", "tests/logs-agent_only.yaml")
	assert.Nil(t, err)
	assert.NotNil(t, config.LogsConfig)

	// valid configuration file
	config, _, err = GetIntegrationConfigFromFile("foo", "tests/testcheck.yaml")
	require.Nil(t, err)
	assert.Equal(t, config.Name, "foo")
	assert.Equal(t, []byte(config.InitConfig), []byte("- test: 21\n"))
	assert.Equal(t, config.Source, "file:tests/testcheck.yaml")
	assert.Equal(t, len(config.Instances), 1)
	assert.Equal(t, []byte(config.Instances[0]), []byte("foo: bar\n"))
	assert.Len(t, config.ADIdentifiers, 0)
	assert.Nil(t, config.MetricConfig)
	assert.Nil(t, config.LogsConfig)

	// autodiscovery
	config, _, err = GetIntegrationConfigFromFile("foo", "tests/ad.yaml")
	require.Nil(t, err)
	assert.Equal(t, config.ADIdentifiers, []string{"foo_id", "bar_id"})

	// advanced autodiscovery
	config, _, err = GetIntegrationConfigFromFile("foo", "tests/advanced_ad.yaml")
	require.Nil(t, err)
	assert.Equal(t, config.AdvancedADIdentifiers, []integration.AdvancedADIdentifier{{KubeService: integration.KubeNamespacedName{Name: "svc-name", Namespace: "svc-ns"}}})

	// advanced autodiscovery kube_endpoints
	config, _, err = GetIntegrationConfigFromFile("foo", "tests/advanced_ad_kube_endpoints.yaml")
	require.Nil(t, err)
	assert.Equal(t,
		[]integration.AdvancedADIdentifier{
			{
				KubeEndpoints: integration.KubeEndpointsIdentifier{
					KubeNamespacedName: integration.KubeNamespacedName{
						Name:      "svc-name",
						Namespace: "svc-ns",
					},
					Resolve: "ip",
				},
			},
		},
		config.AdvancedADIdentifiers,
	)

	// advanced autodiscovery: cel_selector
	config, _, err = GetIntegrationConfigFromFile("foo", "tests/cel_selector.yaml")
	require.Nil(t, err)
	assert.Equal(t, "file:tests/cel_selector.yaml", config.Source)
	expectedRules := workloadfilter.Rules{Containers: []string{"ctn1.rule1", "ctn2.rule2"}}
	assert.Equal(t, expectedRules, config.CELSelector)
	assert.Len(t, config.ADIdentifiers, 0)

	// autodiscovery: check if we correctly refuse to load if a 'docker_images' section is present
	config, _, err = GetIntegrationConfigFromFile("foo", "tests/ad_deprecated.yaml")
	assert.NotNil(t, err)

	// autodiscovery: check that the service ID is ignored when set explicitly.
	// Service ID is not meant to be set in configs provided by users. It's set
	// automatically when needed.
	config, _, err = GetIntegrationConfigFromFile("foo", "tests/ad_with_service_id.yaml")
	assert.Nil(t, err)
	assert.Empty(t, config.ServiceID)
}

func TestReadConfigFiles(t *testing.T) {
	paths := []string{"tests"}
	ResetReader(paths)

	configs, errors, err := ReadConfigFiles(GetAll)
	require.Nil(t, err)
	require.Equal(t, 21, len(configs))
	require.Equal(t, 4, len(errors))

	for _, c := range configs {
		if c.Name == "empty" {
			require.Fail(t, "empty config should not be returned")
		}
	}

	configs, _, err = ReadConfigFiles(WithoutAdvancedAD)
	require.Nil(t, err)
	require.Equal(t, 19, len(configs))

	expectedConfig1 := integration.Config{
		Name: "advanced_ad",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{
			{
				KubeService: integration.KubeNamespacedName{
					Name:      "svc-name",
					Namespace: "svc-ns",
				},
			},
		},
		Instances: []integration.Data{
			integration.Data("foo: bar\n"),
		},
		Source: "file:tests/advanced_ad.yaml",
	}

	expectedConfig2 := integration.Config{
		Name: "advanced_ad_kube_endpoints",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{
			{
				KubeEndpoints: integration.KubeEndpointsIdentifier{
					KubeNamespacedName: integration.KubeNamespacedName{
						Name:      "svc-name",
						Namespace: "svc-ns",
					},
					Resolve: "ip",
				},
			},
		},
		Instances: []integration.Data{
			integration.Data("foo: bar\n"),
		},
		Source: "file:tests/advanced_ad_kube_endpoints.yaml",
	}

	configs, _, err = ReadConfigFiles(WithAdvancedADOnly)
	require.Nil(t, err)
	require.Equal(t, 2, len(configs))

	// Ignore the Source field for comparison because varies by OS
	// Ignore the matchingProgram field for comparison since it's not relevant for the test
	ignoreFields := cmpopts.IgnoreFields(integration.Config{}, "Source", "matchingProgram")

	// Check if expectedConfig1 is in the configs slice
	found := false
	for _, config := range configs {
		if cmp.Equal(config, expectedConfig1, ignoreFields) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expectedConfig not found in configs.\nExpected: %+v\nActual configs: %+v\nDiff: %s",
			expectedConfig1, configs, cmp.Diff(expectedConfig1, configs, ignoreFields))
	}

	// Check if expectedConfig2 is in the configs slice
	found = false
	for _, config := range configs {
		if cmp.Equal(config, expectedConfig2, ignoreFields) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expectedConfig not found in configs.\nExpected: %+v\nActual configs: %+v\nDiff: %s",
			expectedConfig2, configs, cmp.Diff(expectedConfig2, configs, ignoreFields))
	}

	configs, _, err = ReadConfigFiles(func(c integration.Config) bool { return c.Name == "baz" })
	require.Nil(t, err)
	require.Equal(t, 1, len(configs))
	require.Equal(t, configs[0].Name, "baz")
}

func TestReadConfigFilesCache(t *testing.T) {
	testFileContent := `
init_config:
  - this: IsNotOnTheDefaultFile

instances:
  # No configuration is needed for this check.
  - foo: bar`

	tempDir := t.TempDir()
	testFilePath := path.Join(tempDir, "foo.yaml")
	assert.NoError(t, os.WriteFile(testFilePath, []byte(testFileContent), 0o660))

	// Init reader with default config, cache is activated with 5mins TTL
	ResetReader([]string{tempDir})

	// Remove file, Sleep 2s, cache should give us same result
	assert.NoError(t, os.Remove(testFilePath))
	time.Sleep(2 * time.Second)
	configs, errors, err := ReadConfigFiles(GetAll)
	require.Nil(t, err)
	require.Equal(t, 1, len(configs))
	require.Equal(t, 0, len(errors))

	// Change config
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("autoconf_config_files_poll", true)
	mockConfig.SetInTest("autoconf_config_files_poll_interval", 2)

	// Write file + reset reader (trigger a read on all files)
	assert.NoError(t, os.WriteFile(testFilePath, []byte(testFileContent), 0o660))
	ResetReader([]string{tempDir})
	// Verify that we do have the file (hitting the cache)
	configs, errors, err = ReadConfigFiles(GetAll)
	require.Nil(t, err)
	require.Equal(t, 1, len(configs))
	require.Equal(t, 0, len(errors))

	// Remove file, Sleep 2s, we should read again and have nothing
	assert.NoError(t, os.Remove(testFilePath))
	time.Sleep(2 * time.Second)
	configs, errors, err = ReadConfigFiles(GetAll)
	require.Nil(t, err)
	require.Equal(t, 0, len(configs))
	require.Equal(t, 0, len(errors))
}
