// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagentimpl

import (
	"fmt"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config/model"
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

func (ia *inventoryagent) getProvidedConfiguration() (string, error) {
	if !ia.conf.GetBool("inventories_configuration_enabled") {
		return "", fmt.Errorf("inventories_configuration_enabled is disabled")
	}

	return marshalAndScrub(ia.conf.AllSettingsWithoutDefault())
}

func (ia *inventoryagent) getFullConfiguration() (string, error) {
	if !ia.conf.GetBool("inventories_configuration_enabled") {
		return "", fmt.Errorf("inventories_configuration_enabled is disabled")
	}

	return marshalAndScrub(ia.conf.AllSettings())
}

func (ia *inventoryagent) getFileConfiguration() (string, error) {
	if !ia.conf.GetBool("inventories_configuration_enabled") {
		return "", fmt.Errorf("inventories_configuration_enabled is disabled")
	}

	return marshalAndScrub(ia.conf.AllSourceSettingsWithoutDefault(model.SourceFile))
}

func (ia *inventoryagent) getEnvVarConfiguration() (string, error) {
	if !ia.conf.GetBool("inventories_configuration_enabled") {
		return "", fmt.Errorf("inventories_configuration_enabled is disabled")
	}

	return marshalAndScrub(ia.conf.AllSourceSettingsWithoutDefault(model.SourceEnvVar))
}

func (ia *inventoryagent) getRuntimeConfiguration() (string, error) {
	if !ia.conf.GetBool("inventories_configuration_enabled") {
		return "", fmt.Errorf("inventories_configuration_enabled is disabled")
	}

	return marshalAndScrub(ia.conf.AllSourceSettingsWithoutDefault(model.SourceAgentRuntime))
}

func (ia *inventoryagent) getRemoteConfiguration() (string, error) {
	if !ia.conf.GetBool("inventories_configuration_enabled") {
		return "", fmt.Errorf("inventories_configuration_enabled is disabled")
	}

	return marshalAndScrub(ia.conf.AllSourceSettingsWithoutDefault(model.SourceRC))
}

func (ia *inventoryagent) getCliConfiguration() (string, error) {
	if !ia.conf.GetBool("inventories_configuration_enabled") {
		return "", fmt.Errorf("inventories_configuration_enabled is disabled")
	}

	return marshalAndScrub(ia.conf.AllSourceSettingsWithoutDefault(model.SourceCLI))
}

func (ia *inventoryagent) getSourceLocalConfiguration() (string, error) {
	if !ia.conf.GetBool("inventories_configuration_enabled") {
		return "", fmt.Errorf("inventories_configuration_enabled is disabled")
	}

	return marshalAndScrub(ia.conf.AllSourceSettingsWithoutDefault(model.SourceLocalConfigProcess))
}
