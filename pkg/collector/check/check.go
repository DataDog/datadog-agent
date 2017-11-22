// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package check

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	yaml "gopkg.in/yaml.v2"
)

const (
	// DefaultCheckInterval is the interval in seconds the scheduler should apply
	// when no value was provided in Check configuration.
	DefaultCheckInterval time.Duration = 15 * time.Second
)

var (
	tplVarRegex = regexp.MustCompile(`%%.+?%%`)

	tplVars = []string{
		"host",
		"pid",
		"port",
		"container-name",
		"tags",
	}
)

var jmxChecks = [...]string{
	"activemq",
	"activemq_58",
	"cassandra",
	"jmx",
	"solr",
	"tomcat",
	"kafka",
}

// ConfigData contains YAML code
type ConfigData []byte

// ConfigRawMap is the generic type to hold YAML configurations
type ConfigRawMap map[interface{}]interface{}

// ConfigJSONMap is the generic type to hold YAML configurations
type ConfigJSONMap map[string]interface{}

// Config is a generic container for configuration files
type Config struct {
	Name          string       // the name of the check
	Instances     []ConfigData // array of Yaml configurations
	InitConfig    ConfigData   // the init_config in Yaml (python check only)
	MetricConfig  ConfigData   // the metric config in Yaml (jmx check only)
	ADIdentifiers []string     // the list of AutoDiscovery identifiers (optional)
}

// Check is an interface for types capable to run checks
type Check interface {
	Run() error                                    // run the check
	Stop()                                         // stop the check if it's running
	String() string                                // provide a printable version of the check name
	Configure(config, initConfig ConfigData) error // configure the check from the outside
	Interval() time.Duration                       // return the interval time for the check
	ID() ID                                        // provide a unique identifier for every check instance
	GetWarnings() []error                          // return the last warning registered by the check
	GetMetricStats() (map[string]int64, error)     // get metric stats from the sender
}

// Stats holds basic runtime statistics about check instances
type Stats struct {
	CheckName          string
	CheckID            ID
	TotalRuns          uint64
	TotalErrors        uint64
	TotalWarnings      uint64
	Metrics            int64
	Events             int64
	ServiceChecks      int64
	TotalMetrics       int64
	TotalEvents        int64
	TotalServiceChecks int64
	ExecutionTimes     [32]int64 // circular buffer of recent run durations, most recent at [(TotalRuns+31) % 32]
	LastExecutionTime  int64     // most recent run duration, provided for convenience
	LastError          string    // error that occurred in the last run, if any
	LastWarnings       []string  // warnings that occurred in the last run, if any
	UpdateTimestamp    int64     // latest update to this instance, unix timestamp in seconds
	m                  sync.Mutex
}

// NewStats returns a new check stats instance
func NewStats(c Check) *Stats {
	return &Stats{
		CheckID:   c.ID(),
		CheckName: c.String(),
	}
}

// Add tracks a new execution time
func (cs *Stats) Add(t time.Duration, err error, warnings []error, metricStats map[string]int64) {
	cs.m.Lock()
	defer cs.m.Unlock()

	// store execution times in Milliseconds
	tms := t.Nanoseconds() / 1e6
	cs.LastExecutionTime = tms
	cs.ExecutionTimes[cs.TotalRuns%uint64(len(cs.ExecutionTimes))] = tms
	cs.TotalRuns++
	if err != nil {
		cs.TotalErrors++
		cs.LastError = err.Error()
	} else {
		cs.LastError = ""
	}
	cs.LastWarnings = []string{}
	if len(warnings) != 0 {
		for _, w := range warnings {
			cs.TotalWarnings++
			cs.LastWarnings = append(cs.LastWarnings, w.Error())
		}
	}
	cs.UpdateTimestamp = time.Now().Unix()

	if m, ok := metricStats["Metrics"]; ok {
		cs.Metrics = m
		if cs.TotalMetrics <= 1000001 {
			cs.TotalMetrics += m
		}
	}
	if ev, ok := metricStats["Events"]; ok {
		cs.Events = ev
		if cs.TotalEvents <= 1000001 {
			cs.TotalEvents += ev
		}
	}
	if sc, ok := metricStats["ServiceChecks"]; ok {
		cs.ServiceChecks = sc
		if cs.TotalServiceChecks <= 1000001 {
			cs.TotalServiceChecks += sc
		}
	}
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
	var yamlBuff bytes.Buffer

	yamlBuff.Write([]byte("init_config:\n"))
	if c.InitConfig != nil {
		yamlBuff.Write([]byte("- "))
		strInit := strings.Split(string(c.InitConfig[:]), "\n")
		for i, line := range strInit {
			if i > 0 {
				yamlBuff.Write([]byte("  "))
			}
			yamlBuff.Write([]byte(line))
			yamlBuff.Write([]byte("\n"))
		}
	}

	yamlBuff.Write([]byte("instances:\n"))
	for _, instance := range c.Instances {
		strInst := strings.Split(string(instance[:]), "\n")
		yamlBuff.Write([]byte("- "))
		for i, line := range strInst {
			if i > 0 {
				yamlBuff.Write([]byte("  "))
			}
			yamlBuff.Write([]byte(line))
			yamlBuff.Write([]byte("\n"))
		}
	}

	return yamlBuff.String()
}

// IsTemplate returns if the config has AD identifiers and template variables
func (c *Config) IsTemplate() bool {
	// a template must have at least an AD identifier
	if len(c.ADIdentifiers) == 0 {
		return false
	}

	// init_config containing template tags
	if tplVarRegex.Match(c.InitConfig) {
		return true
	}

	// any of the instances containing template tags
	for _, inst := range c.Instances {
		if tplVarRegex.Match(inst) {
			return true
		}
	}

	return false
}

// CollectDefaultMetrics returns if the config is for a JMX check which has collect_default_metrics: true
func (c *Config) CollectDefaultMetrics() bool {
	if !IsConfigJMX(c.String(), c.InitConfig) {
		return false
	}

	rawInitConfig := ConfigRawMap{}
	err := yaml.Unmarshal(c.InitConfig, &rawInitConfig)
	if err != nil {
		return false
	}

	x, ok := rawInitConfig["collect_default_metrics"]
	if !ok {
		return false
	}

	collect, ok := x.(bool)
	if !collect || !ok {
		return false
	}

	return true
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

// IsConfigJMX checks if a certain YAML config is a JMX config
func IsConfigJMX(name string, initConf ConfigData) bool {

	for _, check := range jmxChecks {
		if check == name {
			return true
		}
	}

	rawInitConfig := ConfigRawMap{}
	err := yaml.Unmarshal(initConf, &rawInitConfig)
	if err != nil {
		return false
	}

	x, ok := rawInitConfig["is_jmx"]
	if !ok {
		return false
	}

	isJMX, ok := x.(bool)
	if !isJMX || !ok {
		return false
	}

	return true
}
