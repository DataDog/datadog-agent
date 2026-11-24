// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

var (
	// GlobCmp compares the patterns with the field as globs, a `*` not matching
	// a `/`. It applies to the paths.
	GlobCmp = &OpOverrides{
		GlobPatterns: true,
	}
)
