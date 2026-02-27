// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

// Package inventorychecksimpl implements the inventorychecks component interface.
package inventorychecksimpl

import (
	"fmt"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/jmxfetch"
	"github.com/DataDog/datadog-agent/pkg/status/jmx"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func (ic *inventorychecksImpl) getJMXChecksMetadata() (jmxMetadata map[string][]metadata) {
	jmxMetadata = make(map[string][]metadata)
	jmxIntegrations, err := jmxfetch.GetIntegrations()
	if err != nil {
		ic.log.Warnf("could not get JMX metadata: %v", err)
		return
	}

	// Get JMX status to extract Java version information
	statusData := make(map[string]interface{})
	jmx.PopulateStatus(statusData)
	var jmxFetchVersion, javaRuntimeVersion string
	if jmxStatusInfo, ok := statusData["JMXStatus"]; ok {
		if status, ok := jmxStatusInfo.(jmx.Status); ok {
			jmxFetchVersion, javaRuntimeVersion = status.GetInfo()
		}
	}

	if configsRaw, ok := jmxIntegrations["configs"]; ok {
		configs := configsRaw.(map[string]integration.JSONMap)
		for _, jmxIntegration := range configs {
			jmxName := jmxIntegration["check_name"].(string)
			initConfig := jmxfetch.GetJSONSerializableMap(jmxIntegration["init_config"])
			initConfigYaml, err := yaml.Marshal(initConfig)
			if err != nil {
				ic.log.Warnf("could not marshal JMX init_config for %s: %v", jmxName, err)
				continue
			}

			// Scrub the init_config YAML
			scrubbedInitConfigYaml, err := scrubber.ScrubYaml(initConfigYaml)
			if err != nil {
				ic.log.Warnf("could not scrub JMX init_config for %s: %v", jmxName, err)
				// Return early if scrubbing fails to avoid sending unscrubbed data
				continue
			}

			instances := jmxIntegration["instances"].([]integration.JSONMap)
			for _, instance := range instances {
				instanceConfig := jmxfetch.GetJSONSerializableMap(instance)
				instanceYaml, err := yaml.Marshal(instanceConfig)
				if err != nil {
					ic.log.Warnf("could not marshal JMX instance config for %s: %v", jmxName, err)
					continue
				}

				// Scrub the instance YAML
				scrubbedInstanceYaml, err := scrubber.ScrubYaml(instanceYaml)
				if err != nil {
					ic.log.Warnf("could not scrub JMX instance config for %s: %v", jmxName, err)
					// Continue to next instance if scrubbing fails in order to avoid sending unscrubbed data
					continue
				}

				configHash := fmt.Sprintf("%s-%s-%s", jmxName, fmt.Sprint(instance["host"]), fmt.Sprint(instance["port"]))
				if instance["name"] != nil {
					configHash = fmt.Sprintf("%s:%s", configHash, fmt.Sprint(instance["name"]))
				}

				source, ok := jmxIntegration["config.source"].(string)
				if !ok {
					source = "unknown"
				}
				provider, ok := jmxIntegration["config.provider"].(string)
				if !ok {
					// Default is file
					provider = "file"
				}

				metadataEntry := metadata{
					"init_config":     string(scrubbedInitConfigYaml),
					"instance":        string(scrubbedInstanceYaml),
					"config.provider": provider,
					"config.hash":     configHash,
					"config.source":   source,
				}

				// Add Java version information if available
				if jmxFetchVersion != "" {
					metadataEntry["jmxfetch.version"] = jmxFetchVersion
				}
				if javaRuntimeVersion != "" {
					metadataEntry["java.version"] = javaRuntimeVersion
				}

				jmxMetadata[jmxName] = append(jmxMetadata[jmxName], metadataEntry)
			}
		}
	}
	return jmxMetadata
}
