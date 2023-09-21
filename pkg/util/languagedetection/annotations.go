// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

/*
Package languagedetection provides util type definitions and helper methods for the language detection client and handler
*/
package languagedetection

const (

	// Represents a prefix of the language detection annotations
	AnnotationPrefix string = "apm.datadoghq.com/"
)

func GetLanguageAnnotationKey(containerName string) string {
	return AnnotationPrefix + containerName + ".languages"
}
