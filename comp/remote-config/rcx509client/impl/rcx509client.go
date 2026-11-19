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
	"time"

	cfgcomp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/internal/rctags"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	enabledSetting          = "remote_configuration.x509.enabled"
	debugPingEnabledSetting = "remote_configuration.x509.debug_ping_enabled"
	websocketPath           = "api/v2/ws"
	appName                 = "core-agent"

	// hostTagsRefreshInterval matches the existing Remote Config client's poll
	// interval and the backend's minimum interval between tag updates.
	hostTagsRefreshInterval = time.Minute
)

// Dependencies defines the dependencies for the x509 client component.
type Dependencies struct {
	compdef.In

	Lifecycle compdef.Lifecycle
	Config    cfgcomp.Component
	Logger    log.Component
	Hostname  hostname.Component
	Tagger    option.Option[tagger.Component]
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

	// hostTags builds the current host tags; refreshInterval is how often
	// they are recomputed while running.
	hostTags        func() []string
	refreshInterval time.Duration
	cancelRefresh   context.CancelFunc
	refreshDone     chan struct{}
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
	comp.hostTags = hostTagsGetter(deps)
	comp.refreshInterval = hostTagsRefreshInterval
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
	refreshCtx, cancelRefresh := context.WithCancel(context.Background())
	c.cancelRefresh = cancelRefresh
	c.refreshDone = make(chan struct{})
	go c.runHostTagsRefresh(refreshCtx)

	c.setState(rcx509client.StateRunning)
	c.logger.Info("remote config x509 client started")
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		// Report tags before connecting, so the first connection already has
		// them. This runs here rather than in start because building host tags
		// can block for several seconds right after Agent startup, while their
		// providers retry; they may be sparse until the next refresh.
		c.refreshHostTags()
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

func (c *component) stop(ctx context.Context) error {
	c.setState(rcx509client.StateStopping)

	// Stop refreshing tags before closing, so nothing calls the client after
	// Close.
	if c.cancelRefresh != nil {
		c.cancelRefresh()
		<-c.refreshDone
	}

	// Close has no deadline, so stop waiting when ctx is done; the client
	// keeps shutting down in the background. Pass ctx to Close instead once
	// libdd-rc ships Close(ctx) (DataDog/libdd-rc#351).
	done := make(chan error, 1)
	go func() {
		err := c.client.Close()
		c.wg.Wait()
		done <- err
	}()

	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		c.logger.Warnf("remote config x509 client did not stop in time, finishing in the background: %v", ctx.Err())
		return ctx.Err()
	}
	if err != nil {
		c.setState(rcx509client.StateFailed)
		c.logger.Errorf("unable to stop remote config x509 client: %v", err)
		return err
	}
	c.setState(rcx509client.StateStopped)
	c.logger.Info("remote config x509 client stopped")
	return nil
}

// hostTagsGetter returns a function that builds the tags reported for this
// Agent: the same tags the existing Remote Config client sends, plus
// host:<hostname>, which that client sends as its own request field.
func hostTagsGetter(deps Dependencies) func() []string {
	getTags := rctags.Getter(deps.Config, deps.Tagger)
	return func() []string {
		tags := getTags()
		if name := deps.Hostname.GetSafe(context.Background()); name != "" {
			tags = append(tags, "host:"+name)
		}
		return tags
	}
}

// refreshHostTags hands the current tags to the client. The client only passes
// them on when they changed.
func (c *component) refreshHostTags() {
	if err := c.client.SetHostTags(c.hostTags()); err != nil {
		c.logger.Debugf("remote config x509 client: unable to set host tags: %v", err)
	}
}

// runHostTagsRefresh refreshes the host tags every refreshInterval until ctx
// is done.
func (c *component) runHostTagsRefresh(ctx context.Context) {
	defer close(c.refreshDone)
	ticker := time.NewTicker(c.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.refreshHostTags()
		}
	}
}
