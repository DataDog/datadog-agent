// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package check

import (
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// JMXChecks list of JMXFetch checks supported
var JMXChecks = []string{
	"activemq",
	"activemq_58",
	"cassandra",
	"jmx",
	"solr",
	"tomcat",
	"kafka",
}

// IsJMXConfig checks if a certain YAML config is a JMX config
func IsJMXConfig(name string, initConf integration.Data) bool {

	for _, check := range JMXChecks {
		if check == name {
			return true
		}
	}

	rawInitConfig := integration.RawMap{}
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

// CollectDefaultMetrics returns if the config is for a JMX check which has collect_default_metrics: true
func CollectDefaultMetrics(c integration.Config) bool {
	if !IsJMXConfig(c.String(), c.InitConfig) {
		return false
	}

	rawInitConfig := integration.RawMap{}
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
