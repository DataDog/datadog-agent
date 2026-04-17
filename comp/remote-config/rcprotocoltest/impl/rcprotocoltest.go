// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rcprotocoltestimpl implements the rcprotocoltest component.
package rcprotocoltestimpl

import (
	"context"
	"net/url"

	cfgcomp "github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rcprotocoltest "github.com/DataDog/datadog-agent/comp/remote-config/rcprotocoltest/def"
	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the dependencies for the rcprotocoltest component.
type Requires struct {
	compdef.In

	Lc  compdef.Lifecycle
	Cfg cfgcomp.Component
}

// Provides defines the output of the rcprotocoltest component.
type Provides struct {
	compdef.Out

	Comp rcprotocoltest.Component
}

type protocolTestComponent struct{}

// New creates a new rcprotocoltest component. If RC is disabled or the echo
// test is turned off via config, this is a no-op.
func New(reqs Requires) Provides {
	if !configUtils.IsRemoteConfigEnabled(reqs.Cfg) {
		return Provides{Comp: &protocolTestComponent{}}
	}

	if reqs.Cfg.GetBool("remote_configuration.no_websocket_echo") {
		return Provides{Comp: &protocolTestComponent{}}
	}

	apiKey := reqs.Cfg.GetString("api_key")
	if reqs.Cfg.IsSet("remote_configuration.api_key") {
		apiKey = reqs.Cfg.GetString("remote_configuration.api_key")
	}
	apiKey = configUtils.SanitizeAPIKey(apiKey)
	baseRawURL := configUtils.GetMainEndpoint(reqs.Cfg, "https://config.", "remote_configuration.rc_dd_url")

	baseURL, err := url.Parse(baseRawURL)
	if err != nil {
		log.Errorf("remote config protocol test: unable to parse RC URL: %s", err)
		return Provides{Comp: &protocolTestComponent{}}
	}

	httpClient, err := api.NewHTTPClient(api.Auth{APIKey: apiKey}, reqs.Cfg, baseURL)
	if err != nil {
		log.Errorf("remote config protocol test: unable to create HTTP client: %s", err)
		return Provides{Comp: &protocolTestComponent{}}
	}

	actor := newWebSocketTestActor(httpClient)

	reqs.Lc.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			actor.Start()
			return nil
		},
		OnStop: func(_ context.Context) error {
			actor.Stop()
			return nil
		},
	})

	return Provides{Comp: &protocolTestComponent{}}
}
