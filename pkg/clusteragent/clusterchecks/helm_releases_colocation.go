// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"bytes"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// helmReleasesGroup is the colocation group of the Helm check and of the
	// orchestrator instance that collects Helm releases. Both watch the Helm release
	// ConfigMaps, so on the same runner they share that informer.
	helmReleasesGroup = "helm-releases"

	helmCheckName         = "helm"
	orchestratorCheckName = "orchestrator"

	// The Helm releases ConfigMap collector of the orchestrator check is
	// enabled with this setting, and runs in the instances that collect ConfigMaps.
	helmReleasesConfigMapsSetting = "orchestrator_explorer.helm_releases.configmaps.enabled"
	configMapsCollector           = "configmaps"
)

// helmReleasesColocation keeps the Helm check and the orchestrator instances
// that collect Helm releases on the same runner.
type helmReleasesColocation struct {
	enabled bool
}

func newHelmReleasesColocation(enabled bool) *helmReleasesColocation {
	return &helmReleasesColocation{enabled: enabled}
}

// isEnabled satisfies colocationRule.
func (r *helmReleasesColocation) isEnabled() bool {
	return r.enabled
}

// group satisfies colocationRule.
func (r *helmReleasesColocation) group(config integration.Config) string {
	switch config.Name {
	case helmCheckName:
		return helmReleasesGroup
	case orchestratorCheckName:
		if collectsConfigMaps(config) {
			return helmReleasesGroup
		}
	}
	return ""
}

type orchestratorInstance struct {
	Collectors []string `yaml:"collectors"`
}

// collectsConfigMaps reports whether an orchestrator config has an instance
// that runs the ConfigMap collector, and so the Helm releases collector.
func collectsConfigMaps(config integration.Config) bool {
	for _, instance := range config.Instances {
		// Avoid parsing the YAML of the instances that can't match.
		if !bytes.Contains(instance, []byte(configMapsCollector)) {
			continue
		}

		var parsed orchestratorInstance
		if err := yaml.Unmarshal(instance, &parsed); err != nil {
			log.Debugf("Cannot read the collectors of %s: %v", config.Name, err)
			continue
		}
		for _, collector := range parsed.Collectors {
			// Collectors are named "<resource>" or "<group>/<version>/<resource>",
			// split like the orchestrator check's SplitCollectorName.
			groupVersion, resource := "", collector
			if idx := strings.LastIndex(collector, "/"); idx != -1 {
				groupVersion, resource = collector[:idx], collector[idx+1:]
			}
			if resource == configMapsCollector && (groupVersion == "" || groupVersion == "v1") {
				return true
			}
		}
	}
	return false
}
