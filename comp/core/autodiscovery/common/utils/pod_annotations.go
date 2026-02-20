// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

const (
	// KubeAnnotationPrefix is the prefix used by AD in Kubernetes
	// annotations.
	KubeAnnotationPrefix = "ad.datadoghq.com/"

	legacyPodAnnotationPrefix = "service-discovery.datadoghq.com/"

	podAnnotationFormat       = KubeAnnotationPrefix + "%s."
	legacyPodAnnotationFormat = legacyPodAnnotationPrefix + "%s."

	podAnnotationCheckIDFormat = podAnnotationFormat + checkIDPath
)

// ExtractCheckIDFromPodAnnotations returns whether there is a custom check ID for a given
// container based on the pod annotations
func ExtractCheckIDFromPodAnnotations(annotations map[string]string, containerName string) (string, bool) {
	id, found := annotations[fmt.Sprintf(podAnnotationCheckIDFormat, containerName)]
	return id, found
}

// ExtractCheckNamesFromPodAnnotations returns check names from a map of pod
// annotations. In order of priority, it prefers annotations v2, v1, and
// legacy.
func ExtractCheckNamesFromPodAnnotations(annotations map[string]string, adIdentifier string) ([]string, error) {
	prefix := fmt.Sprintf(podAnnotationFormat, adIdentifier)
	legacyPrefix := fmt.Sprintf(legacyPodAnnotationFormat, adIdentifier)
	return extractCheckNamesFromMap(annotations, prefix, legacyPrefix)
}

// ExtractTemplatesFromAnnotations looks for autodiscovery configurations in
// a map of annotations and returns them if found. In order of priority, it
// prefers annotations v2, v1, and legacy.
func ExtractTemplatesFromAnnotations(entityName string, annotations map[string]string, adIdentifier string) ([]integration.Config, []error) {
	prefix := fmt.Sprintf(podAnnotationFormat, adIdentifier)
	legacyPrefix := fmt.Sprintf(legacyPodAnnotationFormat, adIdentifier)
	res, err := extractTemplatesFromMapWithV2(entityName, annotations, prefix, legacyPrefix)
	return res, err
}

// ValidatePodChecksAnnotation validates the JSON used in Kubernetes pod check
// annotations (e.g. ad.datadoghq.com/<container>.checks). It returns an error
// if the JSON is syntactically invalid or does not match the expected structure.
// Use this to validate annotation content before applying to a pod, or from a
// CLI tool. Invalid JSON causes the autodiscovery check to fail; errors are
// reported in the "Configuration Errors" section of `agent status`.
func ValidatePodChecksAnnotation(checksJSON string) error {
	if checksJSON == "" {
		return fmt.Errorf("pod check annotation JSON is empty")
	}
	var namedChecks map[string]struct {
		Name                    string          `json:"name"`
		InitConfig              json.RawMessage `json:"init_config"`
		Instances               []interface{}   `json:"instances"`
		Logs                    json.RawMessage `json:"logs"`
		IgnoreAutodiscoveryTags bool            `json:"ignore_autodiscovery_tags"`
		CheckTagCardinality     string          `json:"check_tag_cardinality"`
	}
	if err := json.Unmarshal([]byte(checksJSON), &namedChecks); err != nil {
		return fmt.Errorf("invalid JSON in pod check annotation (ad.datadoghq.com/<container>.checks): %w", err)
	}
	// Run the same parsing logic as parseChecksJSON to catch structural errors (e.g. invalid instances).
	_, err := parseChecksJSON("", checksJSON)
	return err
}

// parseChecksJSON parses an AD annotation v2
// (ad.datadoghq.com/redis.checks) JSON string into []integration.Config.
func parseChecksJSON(adIdentifier string, checksJSON string) ([]integration.Config, error) {
	var namedChecks map[string]struct {
		Name                    string          `json:"name"`
		InitConfig              json.RawMessage `json:"init_config"`
		Instances               []interface{}   `json:"instances"`
		Logs                    json.RawMessage `json:"logs"`
		IgnoreAutodiscoveryTags bool            `json:"ignore_autodiscovery_tags"`
		CheckTagCardinality     string          `json:"check_tag_cardinality"`
	}

	err := json.Unmarshal([]byte(checksJSON), &namedChecks)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON in pod check annotation (ad.datadoghq.com/<container>.checks): %w", err)
	}

	checks := make([]integration.Config, 0, len(namedChecks))
	for name, config := range namedChecks {
		if config.Name != "" {
			name = config.Name
		}

		if len(config.InitConfig) == 0 {
			config.InitConfig = json.RawMessage("{}")
		}

		c := integration.Config{
			Name:                    name,
			InitConfig:              integration.Data(config.InitConfig),
			ADIdentifiers:           []string{adIdentifier},
			IgnoreAutodiscoveryTags: config.IgnoreAutodiscoveryTags,
		}

		c.CheckTagCardinality = config.CheckTagCardinality

		if len(config.Logs) > 0 {
			c.LogsConfig = integration.Data(config.Logs)
		}
		for _, i := range config.Instances {
			instance, err := parseJSONObjToData(i)
			if err != nil {
				return nil, err
			}

			c.Instances = append(c.Instances, instance)
		}

		checks = append(checks, c)
	}

	return checks, nil
}
