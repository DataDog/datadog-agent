// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx wires the NDM Remote Configuration provider into autodiscovery.
package fx

import (
	"context"
	"time"

	"go.uber.org/fx"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm"
	ndmdisco "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/discovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/handler"
	ndmsnmp "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/snmp"
	providertypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	ndmdiscovery "github.com/DataDog/datadog-agent/comp/ndmdiscovery/def"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const enabledConfigKey = "network_devices.remote_config.enabled"

// newProvider depends on the discovery component so its lifecycle hook is
// registered, and therefore runs, before the first Update.
func newProvider(lc fx.Lifecycle, cfg config.Component, logComp log.Component, ad autodiscovery.Component, disco ndmdiscovery.Component) (rctypes.ListenerProvider, error) {
	return newListener(lc, cfg, logComp, ad, disco)
}

// configProviderAdder is the one autodiscovery method this package needs.
type configProviderAdder interface {
	AddConfigProvider(providertypes.ConfigProvider, bool, time.Duration)
}

func newListener(lc fx.Lifecycle, cfg config.Component, logComp log.Component, ad configProviderAdder, disco ndmdiscovery.Component) (rctypes.ListenerProvider, error) {
	var listener rctypes.ListenerProvider
	if !configutils.IsRemoteConfigEnabled(cfg) || !cfg.GetBool(enabledConfigKey) {
		// A zero ListenerProvider subscribes to nothing.
		return listener, nil
	}

	provider, err := ndm.NewProvider(logComp, []handler.Handler{
		ndmsnmp.NewHandler(cfg, logComp),
		ndmdisco.NewHandler(disco, logComp),
	})
	if err != nil {
		return listener, err
	}

	// false, 0: the provider streams its changes rather than being polled.
	ad.AddConfigProvider(provider, false, 0)

	// On start, so the features the document drives are already running.
	if path := cfg.GetString(devConfigKey); path != "" {
		lc.Append(fx.Hook{OnStart: func(context.Context) error {
			if err := applyDevConfig(path, provider, logComp); err != nil {
				logComp.Errorf("ndm: %v", err)
			}
			return nil
		}})
	}

	listener.ListenerProvider = rctypes.RCListener{
		data.ProductNDMConfig: provider.Update,
	}
	return listener, nil
}

// Module registers the NDM Remote Configuration provider and listener.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newProvider),
	)
}
