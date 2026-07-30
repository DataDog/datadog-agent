// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender/sdcsender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

// sdcIntervalOverrideCheck wraps a check.Check to override its scheduling
// interval — generalizes the gpu check's own hardcoded
// gpu.collection_interval_override pattern (pkg/collector/corechecks/gpu)
// to any SDC-compressed check, via checks.sdc_compression_interval_override.
type sdcIntervalOverrideCheck struct {
	check.Check
	interval time.Duration
}

func (c *sdcIntervalOverrideCheck) Interval() time.Duration {
	return c.interval
}

// wrapWithSDCIntervalOverride wraps ch to override its scheduling interval
// when checkName is SDC-compressed (per sdcsender.CompressionEnabledFor) and
// checks.sdc_compression_interval_override is set to a positive number of
// seconds, taking precedence over the check's own min_collection_interval.
// Otherwise ch is returned unchanged.
func wrapWithSDCIntervalOverride(ch check.Check, checkName string) check.Check {
	iv := setup.Datadog().GetInt("checks.sdc_compression_interval_override")
	if iv <= 0 || !sdcsender.CompressionEnabledFor(checkName) {
		return ch
	}
	return &sdcIntervalOverrideCheck{Check: ch, interval: time.Duration(iv) * time.Second}
}
