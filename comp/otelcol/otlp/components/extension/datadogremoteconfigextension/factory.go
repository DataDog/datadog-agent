// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2026-present Datadog, Inc.

// Package datadogremoteconfigextension hosts the Agent Remote Configuration
// protocol in the Datadog OpenTelemetry Collector.
package datadogremoteconfigextension

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	rcapi "github.com/DataDog/datadog-agent/pkg/config/remote/api"
	rcservice "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/version"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

// Type is the name used in Collector configuration.
var Type = component.MustNewType("datadog_remoteconfig")

type remoteConfigService interface {
	Start()
	Stop() error
	ClientGetConfigs(context.Context, *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error)
}

// NewFactory reuses the Agent RC engine without a core Agent or IPC connection.
func NewFactory(cfg model.Reader) extension.Factory {
	return extension.NewFactory(Type, createDefaultConfig,
		func(_ context.Context, set extension.Settings, config component.Config) (extension.Extension, error) {
			c := config.(*Config)
			if err := c.Validate(); err != nil {
				return nil, err
			}
			if cfg == nil {
				return nil, errors.New("Collector configuration is required")
			}
			return &remoteConfigExtension{
				config:   c,
				settings: set,
				newService: func(ctx context.Context) (remoteConfigService, error) {
					id := sha256.Sum256([]byte(set.ID.String()))
					cache := filepath.Join(c.CacheDirectory, hex.EncodeToString(id[:]))
					if err := os.MkdirAll(cache, 0700); err != nil {
						return nil, err
					}
					hostname, err := os.Hostname()
					if err != nil {
						return nil, err
					}
					endpoint := "https://config." + c.API.Site
					backendURL, err := url.Parse(endpoint)
					if err != nil {
						return nil, err
					}
					client, err := rcapi.NewHTTPClient(rcapi.Auth{APIKey: string(c.API.Key)}, cfg, backendURL)
					if err != nil {
						return nil, err
					}
					return rcservice.NewService(
						staticConfig{cfg}, "DDOT Remote Config", endpoint,
						hostname, func() []string { return nil }, serviceReporter{}, version.AgentVersion,
						rcservice.WithAPIKey(string(c.API.Key)),
						rcservice.WithAPIClient(boundedAPI{API: client, lifetime: ctx, timeout: 10 * time.Second}),
						rcservice.WithDatabasePath(cache),
						rcservice.WithConfigRootOverride(c.API.Site, ""),
						rcservice.WithDirectorRootOverride(c.API.Site, ""),
						rcservice.WithRefreshInterval(c.RefreshInterval, "datadog_remoteconfig.refresh_interval"),
					)
				},
			}, nil
		}, component.StabilityLevelDevelopment)
}

// Extension credentials are reloaded through Collector component restart, not
// through unrelated core-Agent configuration updates.
type staticConfig struct{ model.Reader }

func (staticConfig) OnUpdate(model.NotificationReceiver) {}

// The draft exposes HTTP server telemetry; RC-specific metrics are follow-up.
type serviceReporter struct{}

func (serviceReporter) IncRateLimit()                              {}
func (serviceReporter) IncTimeout()                                {}
func (serviceReporter) IncConfigSubscriptionsConnectedCounter()    {}
func (serviceReporter) IncConfigSubscriptionsDisconnectedCounter() {}
func (serviceReporter) SetConfigSubscriptionsActive(int)           {}
func (serviceReporter) SetConfigSubscriptionClientsTracked(int)    {}
