// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package metrics

import (
	"encoding/json"
	"fmt"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// keyTagsFromObjectMetadata extracts per-object tags from the upstream DatadogPodAutoscaler CR
// by combining:
//   - the "ad.datadoghq.com/tags" annotation (JSON map of arbitrary key:value tags)
//   - the Unified Service Tagging labels (env, service, version)
//
// Returns nil if no tags are found.
func keyTagsFromObjectMetadata(internal *model.PodAutoscalerInternal) []string {
	if internal == nil {
		return nil
	}
	cr := internal.UpstreamCR()
	if cr == nil {
		return nil
	}
	var tags []string
	if tagsJSON := cr.Annotations[kubernetes.ADTagsAnnotation]; tagsJSON != "" {
		annotationTags, err := parseTagsFromJSON(kubernetes.ADTagsAnnotation, tagsJSON)
		if err != nil {
			log.Debugf("Failed to parse %s annotation for %s/%s: %v", kubernetes.ADTagsAnnotation, cr.Namespace, cr.Name, err)
		} else {
			tags = append(tags, annotationTags...)
		}
	}
	tags = append(tags, kubernetes.GetStandardTags(cr.Labels)...)
	if len(tags) == 0 {
		return nil
	}
	return tags
}

// parseContainerAnnotationTags parses all "ad.datadoghq.com/<container-name>.tags" annotations
// and returns a map keyed by container name, with the annotation-derived tags as values.
// The resource-level "ad.datadoghq.com/tags" annotation is excluded.
func parseContainerAnnotationTags(annotations map[string]string) (map[string][]string, error) {
	result := make(map[string][]string)
	var errs []error
	for annotationKey, tagsJSON := range annotations {
		if !strings.HasPrefix(annotationKey, kubernetes.ADAnnotationPrefix) ||
			annotationKey == kubernetes.ADTagsAnnotation ||
			!strings.HasSuffix(annotationKey, ".tags") {
			continue
		}
		containerName := strings.TrimSuffix(
			strings.TrimPrefix(annotationKey, kubernetes.ADAnnotationPrefix), ".tags")
		if containerName == "" {
			continue
		}
		tags, err := parseTagsFromJSON(annotationKey, tagsJSON)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if len(tags) == 0 {
			continue
		}
		result[containerName] = tags
	}
	return result, k8serrors.NewAggregate(errs)
}

// parseTagsFromJSON parses a JSON map {"key":"val"} or {"key":["v1","v2"]} into []string tags.
// Returns nil on absent input; returns an error for invalid JSON.
func parseTagsFromJSON(annotationKey, tagsJSON string) ([]string, error) {
	if tagsJSON == "" {
		return nil, nil
	}
	var tagMap map[string]any
	if err := json.Unmarshal([]byte(tagsJSON), &tagMap); err != nil {
		return nil, fmt.Errorf("%s annotation has invalid JSON: %w", annotationKey, err)
	}
	var tags []string
	for k, v := range tagMap {
		switch val := v.(type) {
		case string:
			tags = append(tags, k+":"+val)
		case []any:
			for _, elem := range val {
				if s, ok := elem.(string); ok {
					tags = append(tags, k+":"+s)
				}
			}
		}
	}
	return tags, nil
}
