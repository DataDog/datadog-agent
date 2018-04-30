// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package check

import (
	"fmt"
	"hash/fnv"
	"log"
	"regexp"
	"strconv"

	yaml "gopkg.in/yaml.v2"
)

var tplVarRegex = regexp.MustCompile(`%%.+?%%`)

// ConfigData contains YAML code
type ConfigData []byte

// ConfigRawMap is the generic type to hold YAML configurations
type ConfigRawMap map[interface{}]interface{}

// ConfigJSONMap is the generic type to hold JSON configurations
type ConfigJSONMap map[string]interface{}

// Config is a generic container for configuration files
type Config struct {
	Name          string       `json:"check_name"`     // the name of the check
	Instances     []ConfigData `json:"instances"`      // array of Yaml configurations
	InitConfig    ConfigData   `json:"init_config"`    // the init_config in Yaml (python check only)
	MetricConfig  ConfigData   `json:"metric_config"`  // the metric config in Yaml (jmx check only)
	LogsConfig    ConfigData   `json:"log_config"`     // the logs config in Yaml (logs-agent only)
	ADIdentifiers []string     `json:"ad_identifiers"` // the list of AutoDiscovery identifiers (optional)
	Provider      string       `json:"provider"`       // the provider that issued the config
}

// Equal determines whether the passed config is the same
func (c *Config) Equal(config *Config) bool {
	if config == nil {
		return false
	}

	return c.Digest() == config.Digest()
}

// String YAML representation of the config
func (c *Config) String() string {
	rawConfig := make(map[interface{}]interface{})
	var initConfig interface{}
	var instances []interface{}

	yaml.Unmarshal(c.InitConfig, &initConfig)
	rawConfig["init_config"] = initConfig

	for _, i := range c.Instances {
		var instance interface{}
		yaml.Unmarshal(i, &instance)
		instances = append(instances, instance)
	}
	rawConfig["instances"] = instances

	buffer, err := yaml.Marshal(&rawConfig)
	if err != nil {
		log.Fatal(err)
	}

	return string(buffer)
}

// IsTemplate returns if the config has AD identifiers
func (c *Config) IsTemplate() bool {
	return len(c.ADIdentifiers) > 0
}

// AddMetrics adds metrics to a check configuration
func (c *Config) AddMetrics(metrics ConfigData) error {
	var rawInitConfig ConfigRawMap
	err := yaml.Unmarshal(c.InitConfig, &rawInitConfig)
	if err != nil {
		return err
	}

	var rawMetricsConfig []interface{}
	err = yaml.Unmarshal(metrics, &rawMetricsConfig)
	if err != nil {
		return err
	}

	// Grab any metrics currently in init_config
	var conf []interface{}
	currMetrics := make(map[string]bool)
	if _, ok := rawInitConfig["conf"]; ok {
		if currentMetrics, ok := rawInitConfig["conf"].([]interface{}); ok {
			for _, metric := range currentMetrics {
				conf = append(conf, metric)

				if metricS, e := yaml.Marshal(metric); e == nil {
					currMetrics[string(metricS)] = true
				}
			}
		}
	}

	// Add new metrics, skip duplicates
	for _, metric := range rawMetricsConfig {
		if metricS, e := yaml.Marshal(metric); e == nil {
			if !currMetrics[string(metricS)] {
				conf = append(conf, metric)
			}
		}
	}

	// JMX fetch expects the metrics to be a part of init_config, under "conf"
	rawInitConfig["conf"] = conf
	initConfig, err := yaml.Marshal(rawInitConfig)
	if err != nil {
		return err
	}

	c.InitConfig = initConfig
	return nil
}

// GetTemplateVariablesForInstance returns a slice of raw template variables
// it found in a config instance template.
func (c *Config) GetTemplateVariablesForInstance(i int) (vars [][]byte) {
	if len(c.Instances) < i {
		return vars
	}
	return tplVarRegex.FindAll(c.Instances[i], -1)
}

// MergeAdditionalTags merges additional tags to possible existing config tags
func (c *ConfigData) MergeAdditionalTags(tags []string) error {
	rawConfig := ConfigRawMap{}
	err := yaml.Unmarshal(*c, &rawConfig)
	if err != nil {
		return err
	}
	rTags, _ := rawConfig["tags"].([]interface{})
	// convert raw tags to string
	cTags := make([]string, len(rTags))
	for i, t := range rTags {
		cTags[i] = fmt.Sprint(t)
	}
	tagList := append(cTags, tags...)
	if len(tagList) == 0 {
		return nil
	}
	// use set keys to remove duplicate
	tagSet := make(map[string]struct{})
	for _, t := range tagList {
		tagSet[t] = struct{}{}
	}
	// override config tags
	rawConfig["tags"] = []string{}
	for k := range tagSet {
		rawConfig["tags"] = append(rawConfig["tags"].([]string), k)
	}
	// modify original config
	out, err := yaml.Marshal(&rawConfig)
	if err != nil {
		return err
	}
	*c = ConfigData(out)

	return nil
}

// Digest returns an hash value representing the data stored in this configuration
func (c *Config) Digest() string {
	h := fnv.New64()
	h.Write([]byte(c.Name))
	for _, i := range c.Instances {
		h.Write([]byte(i))
	}
	h.Write([]byte(c.InitConfig))
	for _, i := range c.ADIdentifiers {
		h.Write([]byte(i))
	}

	return strconv.FormatUint(h.Sum64(), 16)
}
