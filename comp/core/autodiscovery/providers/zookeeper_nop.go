// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !zk

package providers

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// NewZookeeperConfigProvider returns a new Client connected to a Zookeeper backend.
var NewZookeeperConfigProvider func(providerConfig *config.ConfigurationProviders, telemetryStore *telemetry.Store) (ConfigProvider, error)
