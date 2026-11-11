// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remoteconfig hosts the Agent Remote Configuration service in DDOT.
package remoteconfig

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
	rcservice "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/version"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

type remoteConfigService interface {
	Start()
	Stop() error
	ClientGetConfigs(context.Context, *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error)
}

// New hosts the Agent RC engine without a core Agent or IPC connection.
func New(set extension.Settings, c *Config, cfg model.Reader) (extension.Extension, error) {
	if err := ValidateConfig(c); err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, errors.New("Collector configuration is required")
	}
	reporter, err := newServiceReporter(set)
	if err != nil {
		return nil, err
	}
	e := &remoteConfigExtension{config: c, settings: set}
	e.newService = func(ctx context.Context, host component.Host) (remoteConfigService, error) {
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
		client, closeBackend, err := e.newAPIClient(ctx, host, cfg, backendURL)
		if err != nil {
			return nil, err
		}
		e.closeBackend = closeBackend
		return rcservice.NewService(
			staticConfig{cfg}, "DDOT Remote Config", endpoint,
			hostname, func() []string { return nil }, reporter, version.AgentVersion,
			rcservice.WithAPIKey(string(c.API.Key)),
			rcservice.WithAPIClient(boundedAPI{API: client, lifetime: ctx, timeout: 10 * time.Second}),
			rcservice.WithDatabasePath(cache),
			rcservice.WithConfigRootOverride(c.API.Site, ""),
			rcservice.WithDirectorRootOverride(c.API.Site, ""),
			rcservice.WithRefreshInterval(c.RefreshInterval, "datadog.remote_config.refresh_interval"),
		)
	}
	return e, nil
}

// Extension credentials are reloaded through Collector component restart, not
// through unrelated core-Agent configuration updates.
type staticConfig struct{ model.Reader }

func (staticConfig) OnUpdate(model.NotificationReceiver) {}
