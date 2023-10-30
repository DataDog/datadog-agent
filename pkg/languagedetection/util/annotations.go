// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package util provides util type definitions and helper methods for the language detection client and handler
package util

const (

	// AnnotationPrefix represents a prefix of the language detection annotations
	AnnotationPrefix string = "apm.datadoghq.com/"
)

// GetLanguageAnnotationKey returns the language annotation key for the specified container
func GetLanguageAnnotationKey(containerName string) string {
	return AnnotationPrefix + containerName + ".languages"
}
