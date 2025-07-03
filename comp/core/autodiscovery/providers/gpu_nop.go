// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package providers

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// NewGPUConfigProvider is a no-op for the serverless build, as we don't have GPU support there
var NewGPUConfigProvider func(providerConfig *pkgconfigsetup.ConfigurationProviders, wmeta workloadmeta.Component, telemetry *telemetry.Store) (types.ConfigProvider, error)
