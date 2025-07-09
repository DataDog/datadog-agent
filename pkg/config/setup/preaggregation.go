// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func setupPreaggregation(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("enable_preaggr_pipeline", false)
	config.BindEnvAndSetDefault("preaggr_dd_url", "https://api.datad0g.com")
	config.BindEnv("preaggr_api_key")
}