// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package integration defines types representing an integration configuration,
// which can be used by several components of the agent to configure checks or
// log collectors, for example.
package integration

import (
	"strings"

	logComp "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// ConfigSourceToMetadataMap converts a config source string to a metadata map.
func ConfigSourceToMetadataMap(source string, instance map[string]interface{}) {
	if instance == nil {
		instance = make(map[string]interface{})
	}
	splitSource := strings.SplitN(source, ":", 2)
	instance["config.provider"] = splitSource[0]
	if len(splitSource) > 1 {
		instance["config.source"] = splitSource[1]
	}
}

// ScrubCheckConfig scrubs all secrets in the config.
func ScrubCheckConfig(config Config, logs logComp.Component) Config {
	scrubbedConfig := config
	scrubbedInstances := make([]Data, len(config.Instances))
	for instanceIndex, inst := range config.Instances {
		scrubbedData, err := scrubber.ScrubYaml(inst)
		if err != nil {
			logs.Warnf("error scrubbing secrets from config: %s", err)
			continue
		}
		scrubbedInstances[instanceIndex] = scrubbedData
	}
	scrubbedConfig.Instances = scrubbedInstances

	if len(config.InitConfig) > 0 {
		scrubbedData, err := scrubber.ScrubYaml(config.InitConfig)
		if err != nil {
			logs.Warnf("error scrubbing secrets from init config: %s", err)
			scrubbedConfig.InitConfig = []byte{}
		} else {
			scrubbedConfig.InitConfig = scrubbedData
		}
	}

	if len(config.MetricConfig) > 0 {
		scrubbedData, err := scrubber.ScrubYaml(config.MetricConfig)
		if err != nil {
			logs.Warnf("error scrubbing secrets from metric config: %s", err)
			scrubbedConfig.MetricConfig = []byte{}
		} else {
			scrubbedConfig.MetricConfig = scrubbedData
		}
	}

	if len(config.LogsConfig) > 0 {
		scrubbedData, err := scrubber.ScrubYaml(config.LogsConfig)
		if err != nil {
			logs.Warnf("error scrubbing secrets from logs config: %s", err)
			scrubbedConfig.LogsConfig = []byte{}
		} else {
			scrubbedConfig.LogsConfig = scrubbedData
		}
	}

	return scrubbedConfig

}
