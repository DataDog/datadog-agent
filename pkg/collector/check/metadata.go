// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// GetMetadata returns information about a specific check instances. If 'includeConfig' it true, the instance configuration
// will be scrubbed and included in the returned map
func GetMetadata(c Info, includeConfig bool) map[string]interface{} {
	instance := map[string]interface{}{}

	instanceID := string(c.ID())
	instance["config.hash"] = instanceID

	integration.ConfigSourceToMetadataMap(c.ConfigSource(), instance)

	if includeConfig {
		if instanceScrubbed, err := scrubber.ScrubYamlString(c.InstanceConfig()); err != nil {
			log.Errorf("Could not scrub instance configuration for check id %s: %s", instanceID, err)
		} else {
			instance["instance_config"] = strings.TrimSpace(instanceScrubbed)
		}

		if initScrubbed, err := scrubber.ScrubYamlString(c.InitConfig()); err != nil {
			log.Errorf("Could not scrub init configuration for check id %s: %s", instanceID, err)
		} else {
			instance["init_config"] = strings.TrimSpace(initScrubbed)
		}
	}
	return instance
}
