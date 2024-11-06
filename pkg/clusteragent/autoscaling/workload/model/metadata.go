// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package model

const (
	AnnotationsURLKey         = "autoscaling.datadoghq.com/url"
	AnnotationsFallbackURLKey = "autoscaling.datadoghq.com/fallback-url"
)

// Annotations represents the relevant annotations on a DatadogPodAutoscaler object
type Annotations struct {
	Endpoint         string
	FallbackEndpoint string
}

func ParseAnnotations(annotations map[string]string) Annotations {
	return Annotations{
		Endpoint:         annotations[AnnotationsURLKey],
		FallbackEndpoint: annotations[AnnotationsFallbackURLKey],
	}
}
