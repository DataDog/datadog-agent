// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package model

import "encoding/json"

// exported for testing purposes
const (
	AnnotationsURLKey         = "autoscaling.datadoghq.com/url"
	AnnotationsFallbackURLKey = "autoscaling.datadoghq.com/fallback-url"
	AnnotationsSettingsKey    = "autoscaling.datadoghq.com/settings"
)

// Annotations represents the relevant annotations on a DatadogPodAutoscaler object
type Annotations struct {
	Endpoint         string
	FallbackEndpoint string
	Settings         map[string]string
}

// ParseAnnotations extracts the relevant autoscaling annotations from a kubernetes annotation map
func ParseAnnotations(annotations map[string]string) Annotations {
	annotation := Annotations{
		Endpoint:         annotations[AnnotationsURLKey],
		FallbackEndpoint: annotations[AnnotationsFallbackURLKey],
	}

	settings := map[string]string{}
	err := json.Unmarshal([]byte(annotations[AnnotationsSettingsKey]), &settings)
	if err == nil && len(settings) > 0 {
		annotation.Settings = settings
	}

	return annotation
}
