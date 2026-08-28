// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package credential

import "strings"

// DirectivePrefix identifies a delegated-auth directive in an API key field.
// It is the canonical constant; local copies in subsystems should be replaced
// with this.
const DirectivePrefix = "DELA("

// IsDirective reports whether a value is a DELA(...) directive. Used by
// consumers to detect directives in config values and avoid shipping the
// literal directive text as an API key.
func IsDirective(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), DirectivePrefix)
}
