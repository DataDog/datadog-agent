// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package settings

import (
	"fmt"

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
	return common.DSD.DebugMetricsStats, nil
}

func (s dsdStatsRuntimeSetting) Set(v interface{}) error {
	var newValue bool

	// to be cautious, take care of both calls with a string (cli) or a bool (programmaticaly)
	str, ok := v.(string)
	if !ok {
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("bad parameter provided")
		}
		newValue = b
	} else {
		switch str {
		case "true":
			newValue = true
		case "false":
			newValue = false
		default:
			return fmt.Errorf("bad parameter value provided: %v", str)
		}
	}

	common.DSD.DebugMetricsStats = newValue
	config.Datadog.Set("dogstatsd_metrics_stats_enable", newValue)
	return nil
}
