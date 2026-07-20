// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx wires the Network Path Remote Configuration provider into Autodiscovery.
package fx

import (
	"time"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	networkpathprovider "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/networkpath"
	providertypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/def"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	compdef.In

	Config        config.Component
	Autodiscovery autodiscovery.Component
}

func newProvider(deps dependencies) rctypes.ListenerProvider {
	return newListener(deps.Config, deps.Autodiscovery)
}

type configProviderAdder interface {
	AddConfigProvider(providertypes.ConfigProvider, bool, time.Duration)
}

func newListener(cfg config.Component, autodiscovery configProviderAdder) rctypes.ListenerProvider {
	var listener rctypes.ListenerProvider
	if !configutils.IsRemoteConfigEnabled(cfg) || !cfg.GetBool("network_path.remote_config.enabled") {
		return listener
	}

	provider := networkpathprovider.NewProvider()
	autodiscovery.AddConfigProvider(provider, false, 0)
	listener.ListenerProvider = rctypes.RCListener{
		data.ProductNetworkPath: provider.Update,
	}
	return listener
}

// Module registers the scheduled Network Path RC provider and listener.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(newProvider),
	)
}
