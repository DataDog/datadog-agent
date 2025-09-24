// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func setupPreaggregation(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("preaggregation.enabled", false)
	config.BindEnv("preaggregation.dd_url")  //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	config.BindEnv("preaggregation.api_key") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	config.BindEnvAndSetDefault("preaggregation.metric_allowlist", []string{})
}
