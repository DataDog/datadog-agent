// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package settings

import (
	"fmt"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// dsdStatsRuntimeSetting wraps operations to change log level at runtime.
type dsdStatsRuntimeSetting string

func (s dsdStatsRuntimeSetting) Description() string {
	return "Enable/disable the dogstatsd debug stats. Possible values: true, false"
}

func (s dsdStatsRuntimeSetting) Name() string {
	return string(s)
}

func (s dsdStatsRuntimeSetting) Get() (interface{}, error) {
	return atomic.LoadUint64(&common.DSD.Debug.Enabled) == 1, nil
}

func (s dsdStatsRuntimeSetting) Set(v interface{}) error {
	var newValue bool
	var err error

	if newValue, err = getBool(v); err != nil {
		return fmt.Errorf("dsdStatsRuntimeSetting: %v", err)
	}

	if newValue {
		common.DSD.EnableMetricsStats()
	} else {
		common.DSD.DisableMetricsStats()
	}

	config.Datadog.Set("dogstatsd_metrics_stats_enable", newValue)
	return nil
}
