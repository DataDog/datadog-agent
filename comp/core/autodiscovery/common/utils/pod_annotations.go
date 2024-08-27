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
	fmt.Println("ANDREWQ 9")
	res, err := extractTemplatesFromMapWithV2(entityName, annotations, prefix, legacyPrefix)
	fmt.Println("ANDREWQ 10 ", res)
	return res, err
}

// parseChecksJSON parses an AD annotation v2
// (ad.datadoghq.com/redis.checks) JSON string into []integration.Config.
func parseChecksJSON(adIdentifier string, checksJSON string) ([]integration.Config, error) {
	var namedChecks map[string]struct {
		Name                    string          `json:"name"`
		InitConfig              json.RawMessage `json:"init_config"`
		Instances               []interface{}   `json:"instances"`
		Logs                    []interface{}   `json:"logs"`
		IgnoreAutodiscoveryTags bool            `json:"ignore_autodiscovery_tags"`
	}
	fmt.Println("andrewq", checksJSON)
	err := json.Unmarshal([]byte(checksJSON), &namedChecks)
	if err != nil {
		return nil, fmt.Errorf("cannot parse check configuration: %w", err)
	}
	// docker run -l com.datadoghq.ad.checks="{\"<INTEGRATION_NAME>\": {\"instances\": [<INSTANCE_CONFIG>], \"logs\": [<LOGS_CONFIG>]}}"
	// docker run -l "com.datadoghq.ad.checks="{\"apache\": {\"logs\": [{\"type\":\"file\"}]}}""
	fmt.Println("WACK PRINTING KEY/VALS")

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
		for _, i := range config.Logs {
			log, err := parseJSONObjToData(i)
			// fmt.Println("hickity", log)
			// fmt.Println("hickity", integration.Data("{\"service\":\"any_service\",\"source\":\"any_source\"}"))
			// fmt.Println("hickity", i)
			if err != nil {
				return nil, err
			}

			c.LogsConfig = log
		}
		for _, i := range config.Instances {
			instance, err := parseJSONObjToData(i)
			if err != nil {
				return nil, err
			}

			c.Instances = append(c.Instances, instance)
		}
		fmt.Println("---------------------------")
		fmt.Println("wacktest11", c)
		fmt.Println("LOGS CONFIG IS ", c.LogsConfig)
		fmt.Println("---------------------------")
		checks = append(checks, c)
	}

	/*
		------------------------------------------------------------------------
		[{apache [[123 34 97 112 97 99 104 101 95 115 116 97 116 117 115 95 117 114 108 34 58 34 104 116 116 112 58 47 47 37 37 104 111 115 116 37 37 47 115 101 114 118 101 114 45 115 116 97 116 117 115 63 97 117 116 111 50 34 125]] [123 125] [] [91 10 9 9 9 9 9 9 9 123 34 115 101 114 118 105 99 101 34 58 34 97 110 121 95 115 101 114 118 105 99 101 34 44 32 34 115 111 117 114 99 101 34 58 34 97 110 121 95 115 111 117 114 99 101 34 125 10 9 9 9 9 9 9 93] [docker://foobar] []    false   false false false}]
		------------------------------------------------------------------------
		[{apache [[123 34 97 112 97 99 104 101 95 115 116 97 116 117 115 95 117 114 108 34 58 34 104 116 116 112 58 47 47 37 37 104 111 115 116 37 37 47 115 101 114 118 101 114 45 115 116 97 116 117 115 63 97 117 116 111 50 34 125]] [123 125] [] [] [docker://foobar] []    false   false false false} {apache [] [] [] [91 123 34 115 101 114 118 105 99 101 34 58 34 97 110 121 95 115 101 114 118 105 99 101 34 44 34 115 111 117 114 99 101 34 58 34 97 110 121 95 115 111 117 114 99 101 34 125 93] [docker://foobar] []    false   false false false}]
	*/

	return checks, nil
}
