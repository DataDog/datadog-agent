// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package autodiscovery provides the autodiscovery component for the Datadog Agent
package autodiscovery

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// Component is the component type.
// team: container-platform
type Component interface {
	AddConfigProvider(provider types.ConfigProvider, shouldPoll bool, pollInterval time.Duration)
	LoadAndRun(ctx context.Context)
	GetAllConfigs() []integration.Config
	AddListeners(listenerConfigs []pkgconfigsetup.Listeners)
	AddScheduler(name string, s scheduler.Scheduler, replayConfigs bool)
	RemoveScheduler(name string)
	GetIDOfCheckWithEncryptedSecrets(checkID checkid.ID) checkid.ID
	GetAutodiscoveryErrors() map[string]map[string]types.ErrorMsgSet
	GetProviderCatalog() map[string]types.ConfigProviderFactory
	GetTelemetryStore() *telemetry.Store
	// TODO (component): once cluster agent uses the API component remove this function
	GetConfigCheck() integration.ConfigCheckResponse
}
