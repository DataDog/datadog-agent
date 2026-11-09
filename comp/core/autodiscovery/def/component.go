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
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
)

// Params controls autodiscovery startup behavior. Its zero value keeps preparation
// lazy until LoadAndRun is called. Supplying Params to Fx is optional.
type Params struct {
	// PreloadConfigsOnStart prepares default providers and listeners in the
	// background from the component's startup hook. Enable only when setup's
	// prerequisites are ready at that point. The startup context bounds the
	// subsequent LoadAndRun wait, not setup itself; shutdown joins setup even
	// if that context expires.
	PreloadConfigsOnStart bool
}

// Component is the component type.
// team: container-platform
type Component interface {
	AddConfigProvider(provider types.ConfigProvider, shouldPoll bool, pollInterval time.Duration)

	// LoadAndRun starts default preparation if needed, waits for it, then starts
	// all registered providers, including manually added ones. A canceled wait
	// returns an error without starting providers or canceling preparation.
	// Call after startup and before shutdown. After a successful call it must not
	// be called again, including concurrently. Mocks may omit default preparation.
	LoadAndRun(ctx context.Context) error
	GetUnresolvedConfigs() []integration.Config
	GetAllConfigs() []integration.Config
	AddListeners(listenerConfigs []pkgconfigsetup.Listeners)
	AddScheduler(name string, s scheduler.Scheduler, replayConfigs bool)
	RemoveScheduler(name string)
	GetIDOfCheckWithEncryptedSecrets(checkID checkid.ID) checkid.ID
	GetAutodiscoveryErrors() map[string]map[string]types.ErrorMsgSet
	AddConfigProviderFromCatalog(cp constants.ConfigurationProviders) error
	GetTelemetryStore() *telemetry.Store
	// TODO (component): once cluster agent uses the API component remove this function
	GetConfigCheck() integration.ConfigCheckResponse
}
