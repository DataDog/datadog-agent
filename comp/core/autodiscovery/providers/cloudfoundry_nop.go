// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !clusterchecks

package providers

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// NewCloudFoundryConfigProvider instantiates a new CloudFoundryConfigProvider from given config
var NewCloudFoundryConfigProvider func(providerConfig *pkgconfigsetup.ConfigurationProviders, telemetryStore *telemetry.Store) (ConfigProvider, error)
