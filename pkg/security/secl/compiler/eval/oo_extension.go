// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import "strings"

var (
	extensionSanitizer = func(kind FieldValueType, pattern string) (string, error) {
		if kind == RegexpValueType || strings.HasPrefix(pattern, ".") {
			return pattern, nil
		}

		return "." + pattern, nil
	}

	// ExtensionCmp normalizes file extension values by stripping a leading dot,
	// allowing rules to use either ".txt" or "txt" to match the same extension.
	ExtensionCmp = &OpOverrides{
		StringCmpOpts: StringCmpOpts{
			Sanitize: extensionSanitizer,
		},
	}
)
