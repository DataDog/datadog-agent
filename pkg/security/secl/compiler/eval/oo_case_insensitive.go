// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

var (
	// CaseInsensitiveCmp lower case values before comparing. Important : this operator override doesn't support approvers
	CaseInsensitiveCmp = &OpOverrides{
		StringCmpOpts: StringCmpOpts{
			CaseInsensitive: true,
		},
	}

	// WindowsPathCmp lower case values before comparing, converts patterns to globs, and normalizes path separators. Important : this operator override doesn't support approvers
	WindowsPathCmp = &OpOverrides{
		StringCmpOpts: StringCmpOpts{
			CaseInsensitive:        true,
			PathSeparatorNormalize: true,
		},
		GlobPatterns: true,
	}
)
