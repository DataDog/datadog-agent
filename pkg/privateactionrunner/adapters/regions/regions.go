// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package regions

import "strings"

func GetRegionFromDDSite(ddSite string) string {
	if ddSite == "datadoghq.eu" {
		return "eu1"
	}
	if strings.HasSuffix(ddSite, ".datadoghq.com") {
		region := strings.TrimSuffix(ddSite, ".datadoghq.com")
		return region
	}
	return "us1"
}
