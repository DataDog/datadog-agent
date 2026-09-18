// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fakeintakeconfig contains Pulumi-free fixture defaults.
package fakeintakeconfig

import "github.com/DataDog/datadog-agent/test/fakeintake/version"

// ImageURL resolves the shared pin; callers own profile lookup and lifecycle.
func ImageURL(image, override string) string {
	if override != "" {
		return override
	}
	return image + ":" + version.Tag
}
