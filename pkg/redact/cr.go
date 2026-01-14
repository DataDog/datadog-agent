// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package redact

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ScrubCRManifest scrubs sensitive information from a Custom Resource Manifest
func ScrubCRManifest(r *unstructured.Unstructured, scrubber *DataScrubber) {
	// Scrub spec fields
	if spec, ok := r.Object["spec"]; ok {
		if specMap, ok := spec.(map[string]interface{}); ok {
			shouldRedact := false
			scrubMap(specMap, scrubber, shouldRedact)
			r.Object["spec"] = specMap
		}
	}
}

// scrubMap recursively scrubs sensitive values in a map
func scrubMap(m map[string]interface{}, scrubber *DataScrubber, parentSensitive bool) {
	for k, v := range m {
		lowerK := strings.ToLower(k)
		isSensitiveKey := scrubber.ContainsSensitiveWord(lowerK)
		shouldRedact := parentSensitive || isSensitiveKey

		if shouldRedact {
			if _, ok := v.(string); ok {
				m[k] = redactedSecret
				continue
			}
		}

		if k == "env" {
			if env, ok := v.([]interface{}); ok {
				scrubEnv(env, scrubber, shouldRedact)
			}
			continue
		}

		switch val := v.(type) {
		case map[string]interface{}:
			scrubMap(val, scrubber, shouldRedact)
		case []interface{}:
			scrubSlice(val, scrubber, shouldRedact)
		}
	}
}

// scrubSlice recursively scrubs sensitive values in a slice
func scrubSlice(a []interface{}, scrubber *DataScrubber, parentSensitive bool) {
	for i, item := range a {
		switch item := item.(type) {
		case string:
			if scrubber.ContainsSensitiveWord(item) || parentSensitive {
				a[i] = redactedSecret
			}
		case map[string]interface{}:
			scrubMap(item, scrubber, parentSensitive)
		case []interface{}:
			scrubSlice(item, scrubber, parentSensitive)
		}
	}
}

// scrubEnv scrubs sensitive values in an env slice
func scrubEnv(env []interface{}, scrubber *DataScrubber, parentSensitive bool) {
	for _, item := range env {
		if item, ok := item.(map[string]interface{}); ok {
			if name, ok := item["name"].(string); ok {
				if _, ok := item["value"].(string); ok {
					if scrubber.ContainsSensitiveWord(name) || parentSensitive {
						item["value"] = redactedSecret
					}
				}
			}
		}
	}
}
