// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package autodiscovery fetch information needed to render the 'autodiscovery' section of the status page.
package autodiscovery

import (
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// PopulateStatus populates the status stats
func PopulateStatus(stats map[string]interface{}) {
	stats["adEnabledFeatures"] = config.GetDetectedFeatures()
	if common.AC != nil {
		stats["adConfigErrors"] = common.AC.GetAutodiscoveryErrors()
	}
	stats["filterErrors"] = containers.GetFilterErrors()
}
