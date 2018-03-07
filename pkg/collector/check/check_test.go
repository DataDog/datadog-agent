// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package check

import (
	"crypto/rand"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	yaml "gopkg.in/yaml.v2"
)

func TestConfigEqual(t *testing.T) {
	config := &Config{}
	assert.False(t, config.Equal(nil))

	another := &Config{}
	assert.True(t, config.Equal(another))

	another.Name = "foo"
	assert.False(t, config.Equal(another))
	config.Name = another.Name
	assert.True(t, config.Equal(another))

	another.InitConfig = ConfigData("fooBarBaz")
	assert.False(t, config.Equal(another))
	config.InitConfig = another.InitConfig
	assert.True(t, config.Equal(another))

	another.Instances = []ConfigData{ConfigData("justFoo")}
	assert.False(t, config.Equal(another))
	config.Instances = another.Instances
	assert.True(t, config.Equal(another))

	config.ADIdentifiers = []string{"foo", "bar"}
	assert.False(t, config.Equal(another))
	another.ADIdentifiers = []string{"foo", "bar"}
	assert.True(t, config.Equal(another))
	another.ADIdentifiers = []string{"bar", "foo"}
	assert.False(t, config.Equal(another))
}

func TestString(t *testing.T) {
	config := &Config{}
	assert.False(t, config.Equal(nil))

	config.Name = "foo"
	config.InitConfig = ConfigData("fooBarBaz: test")
	config.Instances = []ConfigData{ConfigData("justFoo")}

	expected := `init_config:
  fooBarBaz: test
instances:
- justFoo
`
	assert.Equal(t, config.String(), expected)
}

func TestMergeAdditionalTags(t *testing.T) {
	config := &Config{}
	assert.False(t, config.Equal(nil))

	config.Name = "foo"
	config.InitConfig = ConfigData("fooBarBaz")
	config.Instances = []ConfigData{ConfigData("tags: [\"foo\", \"foo:bar\"]")}

	config.Instances[0].MergeAdditionalTags([]string{"foo", "bar"})

	rawConfig := ConfigRawMap{}
	err := yaml.Unmarshal(config.Instances[0], &rawConfig)
	assert.Nil(t, err)
	assert.Contains(t, rawConfig["tags"], "foo")
	assert.Contains(t, rawConfig["tags"], "bar")
	assert.Contains(t, rawConfig["tags"], "foo:bar")

	config.Name = "foo"
	config.InitConfig = ConfigData("fooBarBaz")
	config.Instances = []ConfigData{ConfigData("other: foo")}

	config.Instances[0].MergeAdditionalTags([]string{"foo", "bar"})

	rawConfig = ConfigRawMap{}
	err = yaml.Unmarshal(config.Instances[0], &rawConfig)
	assert.Nil(t, err)
	assert.Contains(t, rawConfig["tags"], "foo")
	assert.Contains(t, rawConfig["tags"], "bar")
}

func TestDigest(t *testing.T) {
	config := &Config{}
	assert.Equal(t, 16, len(config.Digest()))
}

func TestCollectDefaultMetrics(t *testing.T) {
	cfg, err := LoadCheck("foo", "testdata/collect_default_false.yaml")
	assert.Nil(t, err)
	assert.False(t, cfg.CollectDefaultMetrics())

	cfg, err = LoadCheck("foo", "testdata/no_metrics/conf.yaml")
	assert.Nil(t, err)
	assert.True(t, cfg.CollectDefaultMetrics())
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

// this is here to prevent compiler optimization on the benchmarking code
var result string

func BenchmarkID(b *testing.B) {
	var id string // store return value to avoid the compiler to strip the function call
	config := &Config{}
	config.InitConfig = make([]byte, 32000)
	rand.Read(config.InitConfig)
	for n := 0; n < b.N; n++ {
		id = config.Digest()
	}
	result = id
}

type config struct {
	InitConfig interface{} `yaml:"init_config"`
	JMXMetrics interface{} `yaml:"jmx_metrics"`
	Instances  []ConfigRawMap
}

func LoadCheck(name, path string) (Config, error) {
	cf := config{}
	config := Config{Name: name}

	yamlFile, err := ioutil.ReadFile(path)
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
