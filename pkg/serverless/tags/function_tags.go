// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package tags

import (
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
)

// GetFunctionTags returns function tags as key:value comma-separated string.
// These are intended to be used as the function tags value in the tracer
// payload sent from serverless environments. Function tags are derived from
// configured tags (DD_TAGS and DD_EXTRA_TAGS).
func GetFunctionTags(cfg pkgconfigmodel.Reader) string {
	configuredTags := configUtils.GetConfiguredTags(cfg, false)
	return strings.Join(configuredTags, ",")
}
