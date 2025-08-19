// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestCollectDefaultMetrics(t *testing.T) {
	cfg, err := LoadCheck("foo", "testdata/collect_default_false.yaml")
	assert.Nil(t, err)
	assert.False(t, CollectDefaultMetrics(cfg))

	cfg, err = LoadCheck("foo", "testdata/no_metrics/conf.yaml")
	assert.Nil(t, err)
	assert.True(t, CollectDefaultMetrics(cfg))
}

func TestAddMetrics(t *testing.T) {
	metricsCfg, err := LoadCheck("foo", "testdata/metrics.yaml")
	assert.Nil(t, err)
	assert.NotNil(t, metricsCfg.MetricConfig)

	// Add metrics to a file which doesn't have any
	config, err := LoadCheck("foo", "testdata/no_metrics/conf.yaml")
	assert.Nil(t, err)
	assert.NotNil(t, config.InitConfig)
	expected, err := LoadCheck("foo", "testdata/no_metrics/expected.yaml")
	assert.Nil(t, err)
	err = config.AddMetrics(metricsCfg.MetricConfig)
	assert.Nil(t, err)
	assert.Equal(t, expected.String(), config.String())

	// Add metrics to a file which already has some
	config, err = LoadCheck("foo", "testdata/has_metrics/conf.yaml")
	assert.Nil(t, err)
	assert.NotNil(t, config.InitConfig)
	err = config.AddMetrics(metricsCfg.MetricConfig)
	assert.Nil(t, err)
	expected, err = LoadCheck("foo", "testdata/has_metrics/expected.yaml")
	assert.Nil(t, err)
	assert.Equal(t, expected.String(), config.String())

	// Add metrics to a file which has some duplicates
	config, err = LoadCheck("foo", "testdata/has_duplicates/conf.yaml")
	assert.Nil(t, err)
	assert.NotNil(t, config.InitConfig)
	err = config.AddMetrics(metricsCfg.MetricConfig)
	assert.Nil(t, err)
	expected, err = LoadCheck("foo", "testdata/has_duplicates/expected.yaml")
	assert.Nil(t, err)
	assert.Equal(t, expected.String(), config.String())
}

type config struct {
	InitConfig interface{} `yaml:"init_config"`
	JMXMetrics interface{} `yaml:"jmx_metrics"`
	Instances  []integration.RawMap
}

func LoadCheck(name, path string) (integration.Config, error) {
	cf := config{}
	config := integration.Config{Name: name}

	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return config, err
	}

	err = yaml.Unmarshal(yamlFile, &cf)
	if err != nil {
		return config, err
	}

	rawInitConfig, _ := yaml.Marshal(cf.InitConfig)
	config.InitConfig = rawInitConfig

	for _, instance := range cf.Instances {
		// at this point the Yaml was already parsed, no need to check the error
		rawConf, _ := yaml.Marshal(instance)
		config.Instances = append(config.Instances, rawConf)
	}

	if cf.JMXMetrics != nil {
		rawMetricConfig, _ := yaml.Marshal(cf.JMXMetrics)
		config.MetricConfig = rawMetricConfig
	}

	return config, err
}
