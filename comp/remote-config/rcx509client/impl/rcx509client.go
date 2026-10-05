// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package rcx509clientimpl implements the Agent's Remote Config x509 client component.
package rcx509clientimpl

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	cfgcomp "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	enabledSetting          = "remote_configuration.x509.enabled"
	debugPingEnabledSetting = "remote_configuration.x509.debug_ping_enabled"
	websocketPath           = "api/v2/ws"
	appName                 = "core-agent"
)

// Dependencies defines the dependencies for the x509 client component.
type Dependencies struct {
	compdef.In

	Lifecycle compdef.Lifecycle
	Config    cfgcomp.Component
	Logger    log.Component
	Factory   rcx509client.ClientFactory `optional:"true"`
}

// Provides defines the output of the x509 client component.
type Provides struct {
	compdef.Out

	Comp rcx509client.Component
}

type component struct {
	mu    sync.RWMutex
	state rcx509client.State

	client rcx509client.Client
	logger log.Component
	wg     sync.WaitGroup
}

// New constructs the x509 client component. It remains inert unless both the
// existing Remote Config service and the separate x509 feature flag are enabled.
func New(deps Dependencies) Provides {
	comp := &component{state: rcx509client.StateDisabled, logger: deps.Logger}
	if !configutils.IsRemoteConfigEnabled(deps.Config) || !deps.Config.GetBool(enabledSetting) {
		return Provides{Comp: comp}
	}

	if deps.Factory == nil {
		comp.state = rcx509client.StateFailed
		deps.Logger.Error("remote config x509 client is enabled but no client factory is available")
		return Provides{Comp: comp}
	}

	clientConfig, err := buildClientConfig(deps.Config)
	if err != nil {
		comp.state = rcx509client.StateFailed
		deps.Logger.Errorf("remote config x509 client configuration failed: %v", err)
		return Provides{Comp: comp}
	}

	client, err := deps.Factory(clientConfig)
	if err != nil {
		comp.state = rcx509client.StateFailed
		deps.Logger.Errorf("remote config x509 client initialization failed: %v", err)
		return Provides{Comp: comp}
	}
	if client == nil {
		comp.state = rcx509client.StateFailed
		deps.Logger.Error("remote config x509 client factory returned a nil client")
		return Provides{Comp: comp}
	}

	comp.client = client
	comp.state = rcx509client.StateInitialized
	deps.Lifecycle.Append(compdef.Hook{
		OnStart: comp.start,
		OnStop:  comp.stop,
	})
	return Provides{Comp: comp}
}

func buildClientConfig(cfg cfgcomp.Component) (rcx509client.ClientConfig, error) {
	apiKey := cfg.GetString("api_key")
	if cfg.IsConfigured("remote_configuration.api_key") {
		apiKey = cfg.GetString("remote_configuration.api_key")
	}
	apiKey = configutils.SanitizeAPIKey(apiKey)

	baseRawURL := configutils.GetMainEndpoint(cfg, "https://config.", "remote_configuration.rc_dd_url")
	baseURL, err := url.Parse(baseRawURL)
	if err != nil {
		return rcx509client.ClientConfig{}, fmt.Errorf("unable to parse Remote Config URL: %w", err)
	}

	// Reuse the existing RC helper so the x509 WebSocket observes the same
	// proxy, dial, TLS, and plaintext safeguards as the polling client.
	rcHTTPClient, err := api.NewHTTPClient(api.Auth{}, cfg, baseURL)
	if err != nil {
		return rcx509client.ClientConfig{}, err
	}
	transport, err := rcHTTPClient.Transport()
	if err != nil {
		return rcx509client.ClientConfig{}, err
	}

	websocketURL := baseURL.JoinPath(websocketPath)
	switch websocketURL.Scheme {
	case "https":
		websocketURL.Scheme = "wss"
	case "http":
		websocketURL.Scheme = "ws"
	default:
		return rcx509client.ClientConfig{}, fmt.Errorf("unsupported Remote Config URL scheme %q", websocketURL.Scheme)
	}

	return rcx509client.ClientConfig{
		URL:              websocketURL.String(),
		AppName:          appName,
		Version:          version.AgentVersion,
		APIKey:           apiKey,
		HTTPClient:       &http.Client{Transport: transport},
		DebugPingEnabled: cfg.GetBool(debugPingEnabledSetting),
	}, nil
}

func (c *component) State() rcx509client.State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *component) setState(state rcx509client.State) {
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
}

func (c *component) start(_ context.Context) error {
	c.setState(rcx509client.StateRunning)
	c.logger.Info("remote config x509 client started")
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		err := c.client.Start()

		c.mu.Lock()
		defer c.mu.Unlock()
		if c.state == rcx509client.StateStopping || c.state == rcx509client.StateStopped {
			return
		}
		c.state = rcx509client.StateFailed
		if err != nil {
			c.logger.Errorf("remote config x509 client stopped unexpectedly: %v", err)
		} else {
			c.logger.Error("remote config x509 client stopped unexpectedly")
		}
	}()
	return nil
}

func (c *component) stop(_ context.Context) error {
	c.setState(rcx509client.StateStopping)
	err := c.client.Close()
	c.wg.Wait()
	if err != nil {
		c.setState(rcx509client.StateFailed)
		c.logger.Errorf("unable to stop remote config x509 client: %v", err)
		return err
	}
	c.setState(rcx509client.StateStopped)
	c.logger.Info("remote config x509 client stopped")
	return nil
}
