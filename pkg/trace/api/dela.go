// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import "strings"

// delaDirectivePrefix marks a value in an `additional_endpoints`-style config list as a
// delegated-auth directive (e.g. "DELA(<org_uuid>, aws)") rather than a literal API key. Mirrors
// pkg/config/utils.IsDelaDirective's prefix check, duplicated locally so this deliberately lean,
// OTel-Collector-vendored module doesn't pull in pkg/config/setup (and its transitive
// comp/core/delegatedauth dependency) for a single prefix comparison.
const delaDirectivePrefix = "DELA("

// isDelaDirective reports whether a value is a delegated-auth directive rather than a literal
// API key. Callers must skip (not send) values where this returns true.
func isDelaDirective(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), delaDirectivePrefix)
}
