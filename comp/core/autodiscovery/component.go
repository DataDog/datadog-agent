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
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// Component is the component type.
// team: container-platform
type Component interface {
	AddConfigProvider(provider providers.ConfigProvider, shouldPoll bool, pollInterval time.Duration)
	LoadAndRun(ctx context.Context)
	ForceRanOnceFlag()
	HasRunOnce() bool
	GetAllConfigs() []integration.Config
	AddListeners(listenerConfigs []config.Listeners)
	AddScheduler(name string, s scheduler.Scheduler, replayConfigs bool)
	RemoveScheduler(name string)
	MapOverLoadedConfigs(f func(map[string]integration.Config))
	LoadedConfigs() []integration.Config
	GetUnresolvedTemplates() map[string][]integration.Config
	GetIDOfCheckWithEncryptedSecrets(checkID checkid.ID) checkid.ID
	GetAutodiscoveryErrors() map[string]map[string]providers.ErrorMsgSet
	GetProviderCatalog() map[string]providers.ConfigProviderFactory
	// TODO (component): deprecate start/stop methods
	Start()
	Stop()
	// TODO (component): once cluster agent uses the API component remove this function
	GetConfigCheck(verbose bool) []byte
	IsStarted() bool
}
