// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package remoteconfiguration fetch information needed to render the 'remoteconfiguration' section of the status page.
package remoteconfiguration

import (
	"encoding/json"
	"expvar"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// PopulateStatus populates the status stats
func PopulateStatus(stats map[string]interface{}) {
	status := make(map[string]interface{})

	if config.IsRemoteConfigEnabled(config.Datadog) && expvar.Get("remoteConfigStatus") != nil {
		remoteConfigStatusJSON := expvar.Get("remoteConfigStatus").String()
		json.Unmarshal([]byte(remoteConfigStatusJSON), &status) //nolint:errcheck
	} else {
		if !config.Datadog.GetBool("remote_configuration.enabled") {
			status["disabledReason"] = "it is explicitly disabled in the agent configuration. (`remote_configuration.enabled: false`)"
		} else if config.Datadog.GetBool("fips.enabled") {
			status["disabledReason"] = "it is not supported when FIPS is enabled. (`fips.enabled: true`)"
		} else if config.Datadog.GetString("site") == "ddog-gov.com" {
			status["disabledReason"] = "it is not supported on GovCloud. (`site: \"ddog-gov.com\"`)"
		}
	}

	stats["remoteConfiguration"] = status
}
