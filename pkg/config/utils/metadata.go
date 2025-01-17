// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"gopkg.in/yaml.v2"
)

func marshalAndScrub(data interface{}) (string, error) {
	flareScrubber := scrubber.NewWithDefaults()

	provided, err := yaml.Marshal(data)
	if err != nil {
		return "", log.Errorf("could not marshal agent configuration: %s", err)
	}

	scrubbed, err := flareScrubber.ScrubYaml(provided)
	if err != nil {
		return "", log.Errorf("could not scrubb agent configuration: %s", err)
	}

	return string(scrubbed), nil
}

// InjectConfigLayersForMetadata insert into a map all the configuration layers into a map.
//
// This is a common behavior used by many metadata payloads.
func InjectConfigLayersForMetadata(conf config.Reader, configLayers map[model.Source]interface{}, metadata map[string]interface{}) {
	if conf.GetBool("inventories_configuration_enabled") {
		log.Info("configuration in metadata is disabled (see 'inventories_configuration_enabled')")
	}

	layersName := map[model.Source]string{
		model.SourceFile:               "file_configuration",
		model.SourceEnvVar:             "environment_variable_configuration",
		model.SourceFleetPolicies:      "fleet_policies_configuration",
		model.SourceAgentRuntime:       "agent_runtime_configuration",
		model.SourceLocalConfigProcess: "source_local_configuration",
		model.SourceRC:                 "remote_configuration",
		model.SourceCLI:                "cli_configuration",
		model.SourceProvided:           "provided_configuration",
	}

	for source, conf := range configLayers {
		if layer, ok := layersName[source]; ok {
			if yaml, err := marshalAndScrub(conf); err == nil {
				metadata[layer] = yaml
			}
		}
	}
	if yaml, err := marshalAndScrub(conf.AllSettings()); err == nil {
		metadata["full_configuration"] = yaml
	}
}
