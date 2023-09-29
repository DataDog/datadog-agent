// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventories

import (
	"fmt"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

func marshalAndScrub(conf map[string]interface{}) (string, error) {
	flareScrubber := scrubber.NewWithDefaults()

	provided, err := yaml.Marshal(conf)
	if err != nil {
		return "", log.Errorf("could not marshal agent configuration: %s", err)
	}

	scrubbed, err := flareScrubber.ScrubYaml(provided)
	if err != nil {
		return "", log.Errorf("could not scrubb agent configuration: %s", err)
	}

	return string(scrubbed), nil
}

func getFullAgentConfiguration() (string, error) {
	if !config.Datadog.GetBool("inventories_configuration_enabled") {
		return "", fmt.Errorf("inventories_configuration_enabled is disabled")
	}

	return marshalAndScrub(config.Datadog.AllSettings())
}

func getProvidedAgentConfiguration() (string, error) {
	if !config.Datadog.GetBool("inventories_configuration_enabled") {
		return "", fmt.Errorf("inventories_configuration_enabled is disabled")
	}

	return marshalAndScrub(config.Datadog.AllSettingsWithoutDefault())
}

func getAgentYamlConfiguration() (string, error) {
	if !config.Datadog.GetBool("inventories_configuration_enabled") {
		return "", fmt.Errorf("inventories_configuration_enabled is disabled")
	}

	return marshalAndScrub(config.Datadog.AllYamlSettingsWithoutDefault())
}

func getAgentEnvVarConfiguration() (string, error) {
	if !config.Datadog.GetBool("inventories_configuration_enabled") {
		return "", fmt.Errorf("inventories_configuration_enabled is disabled")
	}

	return marshalAndScrub(config.Datadog.AllEnvVarSettingsWithoutDefault())
}

func getAgentSelfConfiguration() (string, error) {
	if !config.Datadog.GetBool("inventories_configuration_enabled") {
		return "", fmt.Errorf("inventories_configuration_enabled is disabled")
	}

	return marshalAndScrub(config.Datadog.AllSelfSettingsWithoutDefault())
}

func getAgentRemoteConfiguration() (string, error) {
	if !config.Datadog.GetBool("inventories_configuration_enabled") {
		return "", fmt.Errorf("inventories_configuration_enabled is disabled")
	}

	return marshalAndScrub(config.Datadog.AllRemoteSettingsWithoutDefault())
}

func getAgentCliConfiguration() (string, error) {
	if !config.Datadog.GetBool("inventories_configuration_enabled") {
		return "", fmt.Errorf("inventories_configuration_enabled is disabled")
	}

	return marshalAndScrub(config.Datadog.AllCliSettingsWithoutDefault())
}
