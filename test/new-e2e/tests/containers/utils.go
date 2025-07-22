// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DatadogPodAutoscalerGVR represents the GroupVersionResource for DatadogPodAutoscaler
var DatadogPodAutoscalerGVR = schema.GroupVersionResource{
	Group:    "datadoghq.com",
	Version:  "v1alpha2",
	Resource: "datadogpodautoscalers",
}

// convertDatadogPodAutoscalerToUnstructured converts a DatadogPodAutoscaler to an unstructured object
func convertDatadogPodAutoscalerToUnstructured(autoscaler *datadoghq.DatadogPodAutoscaler) (*unstructured.Unstructured, error) {
	autoscalerJSON, err := json.Marshal(autoscaler)
	if err != nil {
		return nil, err
	}

	var autoscalerMap map[string]interface{}
	err = json.Unmarshal(autoscalerJSON, &autoscalerMap)
	if err != nil {
		return nil, err
	}

	unstructuredAutoscaler := &unstructured.Unstructured{}
	unstructuredAutoscaler.SetUnstructuredContent(autoscalerMap)

	return unstructuredAutoscaler, nil
}

func assertTags(actualTags []string, expectedTags []*regexp.Regexp, optionalTags []*regexp.Regexp, acceptUnexpectedTags bool) error {
	missingTags := make([]*regexp.Regexp, len(expectedTags))
	copy(missingTags, expectedTags)
	unexpectedTags := []string{}

	for _, actualTag := range actualTags {
		found := false
		for i, expectedTag := range missingTags {
			if expectedTag.MatchString(actualTag) {
				found = true
				missingTags[i] = missingTags[len(missingTags)-1]
				missingTags = missingTags[:len(missingTags)-1]
				break
			}
		}

		if !found {
			for _, optionalTag := range optionalTags {
				if optionalTag.MatchString(actualTag) {
					found = true
					break
				}
			}
		}

		if !found {
			unexpectedTags = append(unexpectedTags, actualTag)
		}
	}

	if (len(unexpectedTags) > 0 && !acceptUnexpectedTags) || len(missingTags) > 0 {
		errs := make([]error, 0, 2)
		if len(unexpectedTags) > 0 {
			errs = append(errs, fmt.Errorf("unexpected tags: %s", strings.Join(unexpectedTags, ", ")))
		}
		if len(missingTags) > 0 {
			errs = append(errs, fmt.Errorf("missing tags: %s", strings.Join(lo.Map(missingTags, func(re *regexp.Regexp, _ int) string { return re.String() }), ", ")))
		}
		return errors.Join(errs...)
	}

	return nil
}
