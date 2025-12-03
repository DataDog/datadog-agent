// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

import "strings"

// ImageValidator provides a validation struct for container images.
type ImageValidator struct {
	raw      string
	name     string
	registry string
	tag      string
}

// NewImageValidator takes an image string provided from a container spec and initializes a new ImageValidator.
func NewImageValidator(i string) *ImageValidator {
	// gcr.io/datadoghq/dd-lib-java-init:v1
	parts := strings.Split(i, ":")
	if len(parts) != 2 {
		return nil
	}

	fullImage := parts[0]
	tag := parts[1]

	// gcr.io/datadoghq/dd-lib-java-init
	parts = strings.Split(fullImage, "/")
	if len(parts) < 1 {
		return nil
	}
	name := parts[len(parts)-1]
	registry := strings.TrimSuffix(fullImage, "/"+name)

	return &ImageValidator{
		raw:      i,
		name:     name,
		registry: registry,
		tag:      tag,
	}
}
