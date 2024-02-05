// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package snmptraps fetch information needed to render the 'snmptraps' section of the status page.
package snmptraps

import (
	trapsconfig "github.com/DataDog/datadog-agent/comp/snmptraps/config"
	"github.com/DataDog/datadog-agent/comp/snmptraps/status/statusimpl"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// PopulateStatus populates the status stats
func PopulateStatus(stats map[string]interface{}) {
	if trapsconfig.IsEnabled(config.Datadog) {
		stats["snmpTrapsStats"] = statusimpl.GetStatus()
	}
}
