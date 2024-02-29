// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagger implements the Tagger component. The Tagger is the central
// source of truth for client-side entity tagging. It runs Collectors that
// detect entities and collect their tags. Tags are then stored in memory (by
// the TagStore) and can be queried by the tagger.Tag() method.

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
// team: container-integrations
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
	IsStarted() bool
}
