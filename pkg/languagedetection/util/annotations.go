// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package util provides util type definitions and helper methods for the language detection client and handler
package util

import (
	"regexp"
)

const (

	// AnnotationPrefix represents a prefix of the language detection annotations
	AnnotationPrefix string = "internal.dd.datadoghq.com/"
)

// AnnotationRegex defines the regex pattern of language detection annotations
var AnnotationRegex = regexp.MustCompile(`internal\.dd\.datadoghq\.com\/(init\.)?(.+?)\.detected_langs`)

// GetLanguageAnnotationKey returns the language annotation key for the specified container
func GetLanguageAnnotationKey(containerName string) string {
	return AnnotationPrefix + containerName + ".detected_langs"
}

// ExtractContainerFromAnnotationKey extracts container name from annotation key and indicates if it is an init container
// if the annotation key is not a language annotation it returns an empty container name
func ExtractContainerFromAnnotationKey(annotationKey string) (string, bool) {
	matches := AnnotationRegex.FindStringSubmatch(annotationKey)
	if len(matches) != 3 {
		return "", false
	}

	containerName := matches[2]

	isInitContainer := matches[1] != ""

	return containerName, isInitContainer
}
