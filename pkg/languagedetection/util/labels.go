// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package util provides util type definitions and helper methods for the language detection client and handler
package util

const (
	// LanguageDetectionEnabledLabel specifies if language detection is enabled for the associated resource or not
	LanguageDetectionEnabledLabel string = "internal.dd.datadoghq.com/language_detection.enabled"
)
