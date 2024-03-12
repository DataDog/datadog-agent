// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	agentconfig "github.com/DataDog/datadog-agent/pkg/config"
)

// IsJMXConfig checks if a certain YAML config contains at least one instance of a JMX config
func IsJMXConfig(config integration.Config) bool {
	for _, instance := range config.Instances {
		if IsJMXInstance(config.Name, instance, config.InitConfig) {
			return true
		}
	}

	return false
}

// IsJMXInstance checks if a certain YAML instance is a JMX config
func IsJMXInstance(name string, instance integration.Data, initConfig integration.Data) bool {
	if _, ok := agentconfig.StandardJMXIntegrations[name]; ok {
		return true
	}

	rawInstance := integration.RawMap{}
	err := yaml.Unmarshal(instance, &rawInstance)
	if err != nil {
		return false
	}

	x, ok := rawInstance["loader"]
	if ok {
		loaderName, ok := x.(string)
		if ok {
			return loaderName == "jmx"
		}
	}

	x, ok = rawInstance["is_jmx"]
	if ok {
		isInstanceJMX, ok := x.(bool)
		if ok && isInstanceJMX {
			return true
		}
	}

	rawInitConfig := integration.RawMap{}
	err = yaml.Unmarshal(initConfig, &rawInitConfig)
	if err != nil {
		return false
	}

	x, ok = rawInitConfig["loader"]
	if ok {
		loaderName, ok := x.(string)
		if ok {
			return loaderName == "jmx"
		}
	}

	x, ok = rawInitConfig["is_jmx"]
	if !ok {
		return false
	}

	isInitConfigJMX, ok := x.(bool)
	if !ok {
		return false
	}

	return isInitConfigJMX
}

// CollectDefaultMetrics returns if the config is for a JMX check which has collect_default_metrics: true
func CollectDefaultMetrics(c integration.Config) bool {
	if !IsJMXConfig(c) {
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
