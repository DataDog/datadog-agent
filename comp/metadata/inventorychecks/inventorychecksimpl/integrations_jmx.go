// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

// Package inventorychecksimpl implements the inventorychecks component interface.
package inventorychecksimpl

import (
	"fmt"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/jmxfetch"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func (ic *inventorychecksImpl) getJMXChecksMetadata() (jmxMetadata map[string][]metadata) {
	jmxMetadata = make(map[string][]metadata)
	jmxIntegrations, err := jmxfetch.GetIntegrations()
	if err != nil {
		ic.log.Warnf("could not get JMX metadata: %v", err)
		return
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
			instances := jmxIntegration["instances"].([]integration.JSONMap)
			for _, instance := range instances {
				configHash := jmxName
				instanceConfig := jmxfetch.GetJSONSerializableMap(instance)
				instanceYaml, err := yaml.Marshal(instanceConfig)
				if err != nil {
					ic.log.Warnf("could not marshal JMX instance config for %s: %v", jmxName, err)
					continue
				}
				if instance["name"] != nil {
					configHash = fmt.Sprintf("%s:%s", jmxName, fmt.Sprint(instance["name"]))
				}

				jmxMetadata[jmxName] = append(jmxMetadata[jmxName], metadata{
					"init_config":     string(initConfigYaml),
					"instance":        string(instanceYaml),
					"config.provider": "file",
					"config.hash":     configHash,
				})
			}
		}
	}
	return jmxMetadata
}
