// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ntp fetch information needed to render the 'ntp' section of the status page.
package ntp

import (
	"expvar"
	"strconv"
)

// PopulateStatus populates the status stats
func PopulateStatus(stats map[string]interface{}) error {
	ntpOffset := expvar.Get("ntpOffset")
	if ntpOffset != nil && ntpOffset.String() != "" {
		float, err := strconv.ParseFloat(expvar.Get("ntpOffset").String(), 64)
		stats["ntpOffset"] = float

		return err
	}

	return nil
}
