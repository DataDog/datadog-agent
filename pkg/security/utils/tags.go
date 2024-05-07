// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"strings"
)

// GetTagValue returns the value of the given tag in the given list
func GetTagValue(tagName string, tags []string) string {
	for _, tag := range tags {
		key, value, found := strings.Cut(tag, ":")
		if !found {
			continue
		}

		if key == tagName {
			return value
		}
	}
	return ""
}

// GetTagName returns the key of a tag in the tag_name:tag_value format
func GetTagName(tag string) string {
	key, _, found := strings.Cut(tag, ":")
	if !found {
		return ""
	}

	return key
}

// GetNameFromTags returns the name inferred from the specified tags
func GetNameFromTags(tags []string) string {
	name := GetTagValue("image_name", tags)
	if name == "" {
		name = GetTagValue("service", tags)
	}
	return name
}

// GetVersionFromTags returns the version inferred from the specified tags
func GetVersionFromTags(tags []string) string {
	tag := GetTagValue("image_tag", tags)
	if tag == "" {
		tag = GetTagValue("version", tags)
	}
	return tag
}
