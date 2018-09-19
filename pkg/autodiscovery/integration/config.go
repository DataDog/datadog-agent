// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package integration

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var tplVarRegex = regexp.MustCompile(`%%.+?%%`)

// Data contains YAML code
type Data []byte

// RawMap is the generic type to hold YAML configurations
type RawMap map[interface{}]interface{}

// JSONMap is the generic type to hold JSON configurations
type JSONMap map[string]interface{}

// CreationTime represents the moment when the service was launched compare to the agent start.
type CreationTime int

const (
	// Before indicates the service was launched before the agent start
	Before CreationTime = iota
	// After indicates the service was launched after the agent start
	After
)

// Config is a generic container for configuration files
type Config struct {
	Name          string       `json:"check_name"`     // the name of the check
	Instances     []Data       `json:"instances"`      // array of Yaml configurations
	InitConfig    Data         `json:"init_config"`    // the init_config in Yaml (python check only)
	MetricConfig  Data         `json:"metric_config"`  // the metric config in Yaml (jmx check only)
	LogsConfig    Data         `json:"logs"`           // the logs config in Yaml (logs-agent only)
	ADIdentifiers []string     `json:"ad_identifiers"` // the list of AutoDiscovery identifiers (optional)
	Provider      string       `json:"provider"`       // the provider that issued the config
	Entity        string       `json:"-"`              // the id of the entity (optional)
	ClusterCheck  bool         `json:"-"`              // cluster-check configuration flag, don't expose in JSON
	CreationTime  CreationTime `json:"-"`              // creation time of service
}

// CommonInstanceConfig holds the reserved fields for the yaml instance data
type CommonInstanceConfig struct {
	MinCollectionInterval int  `yaml:"min_collection_interval"`
	EmptyDefaultHostname  bool `yaml:"empty_default_hostname"`
}

// Equal determines whether the passed config is the same
func (c *Config) Equal(cfg *Config) bool {
	if cfg == nil {
		return false
	}

	return c.Digest() == cfg.Digest()
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
		log.Error(err)
	}

	return string(buffer)
}

// IsTemplate returns if the config has AD identifiers
func (c *Config) IsTemplate() bool {
	return len(c.ADIdentifiers) > 0
}

// AddMetrics adds metrics to a check configuration
func (c *Config) AddMetrics(metrics Data) error {
	var rawInitConfig RawMap
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
func (c *Data) MergeAdditionalTags(tags []string) error {
	rawConfig := RawMap{}
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
	*c = Data(out)

	return nil
}

// Digest returns an hash value representing the data stored in this configuration.
// The ClusterCheck field is intentionally left out to keep a stable digest
// between the cluster-agent and the node-agents
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
