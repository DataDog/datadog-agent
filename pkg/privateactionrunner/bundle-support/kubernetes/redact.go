// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package kubernetes

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-agent/pkg/redact"
)

// ScrubResourceOutput returns a redacted copy of a Kubernetes action's output.
// Keeping the output unstructured allows placeholders in encoded fields such
// as ConfigMap binaryData without decoding them back into typed API objects.
func ScrubResourceOutput(output interface{}) (map[string]interface{}, error) {
	fields, err := runtime.DefaultUnstructuredConverter.ToUnstructured(output)
	if err != nil {
		return nil, fmt.Errorf("could not convert Kubernetes output for redaction: %w", err)
	}
	scrubber := redact.NewDefaultDataScrubber()
	if items, ok := fields["items"].([]interface{}); ok {
		// Scrub list entries separately so each resource keeps its identity and
		// the list's pagination metadata remains intact.
		for _, item := range items {
			resource, ok := item.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("unexpected Kubernetes list item type %T", item)
			}
			scrubResource(resource, scrubber)
		}
	} else {
		scrubResource(fields, scrubber)
	}
	return fields, nil
}

func scrubResource(fields map[string]interface{}, scrubber *redact.DataScrubber) {
	manifest := &unstructured.Unstructured{Object: fields}
	annotations, labels := manifest.GetAnnotations(), manifest.GetLabels()
	redact.RemoveSensitiveAnnotationsAndLabels(annotations, labels)
	for key, value := range annotations {
		annotations[key] = scrubber.ScrubAnnotationValue(value)
	}
	manifest.SetAnnotations(annotations)
	manifest.SetLabels(labels)
	redact.ScrubCRManifest(manifest, scrubber)
}
