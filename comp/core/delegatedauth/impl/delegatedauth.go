// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package delegatedauthimpl implements the delegatedauth component interface
package delegatedauthimpl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	delegatedauthpkg "github.com/DataDog/datadog-agent/pkg/delegatedauth"
	"github.com/DataDog/datadog-agent/pkg/delegatedauth/cloudauth"
)

type delegatedAuthComponent struct {
	config config.Component
	log    log.Component

	mu              sync.RWMutex
	apiKey          *string
	provider        delegatedauthpkg.Provider
	authConfig      *delegatedauthpkg.AuthConfig
	refreshInterval time.Duration

	// Context and cancellation for background refresh goroutine
	refreshCtx    context.Context
	refreshCancel context.CancelFunc
}

type dependencies struct {
	fx.In
	Config config.Component
	Log    log.Component
	Lc     fx.Lifecycle
}

// NewDelegatedAuth creates a new delegated auth Component based on the current configuration
func NewDelegatedAuth(deps dependencies) (delegatedauth.Component, error) {
	if !deps.Config.GetBool("delegated_auth.enabled") {
		return &noopDelegatedAuth{}, nil
	}

	provider := deps.Config.GetString("delegated_auth.provider")
	orgUUID := deps.Config.GetString("delegated_auth.org_uuid")
	refreshInterval := deps.Config.GetDuration("delegated_auth.refresh_interval_mins") * time.Minute

	if refreshInterval == 0 {
		// Default to 60 minutes if refresh interval was set incorrectly
		refreshInterval = 60 * time.Minute
		deps.Log.Warn("Refresh interval was set to 0 defaulting to 60 minutes")
	}

	if orgUUID == "" {
		return nil, fmt.Errorf("delegated_auth.org_uuid is required when delegated_auth.enabled is true")
	}

	var tokenProvider delegatedauthpkg.Provider
	switch provider {
	case cloudauth.ProviderAWS:
		tokenProvider = &cloudauth.AWSAuth{
			AwsRegion: deps.Config.GetString("delegated_auth.aws_region"),
		}
	default:
		return nil, fmt.Errorf("unsupported delegated auth provider: %s", provider)
	}

	authConfig := &delegatedauthpkg.AuthConfig{
		OrgUUID:      orgUUID,
		Provider:     provider,
		ProviderAuth: tokenProvider,
	}

	comp := &delegatedAuthComponent{
		config:          deps.Config,
		log:             deps.Log,
		provider:        tokenProvider,
		authConfig:      authConfig,
		refreshInterval: refreshInterval,
	}

	// Register lifecycle hooks to ensure API key is fetched early
	deps.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			comp.log.Info("Delegated authentication is enabled, fetching initial API key...")

			// Create a context for the background refresh goroutine
			comp.refreshCtx, comp.refreshCancel = context.WithCancel(context.Background())

			// Fetch the initial API key synchronously during startup
			apiKey, err := comp.GetAPIKey(ctx)
			if err != nil {
				comp.log.Errorf("Failed to get initial delegated API key: %v", err)
				// Return nil here to not stop the agent from starting
				return nil
			}

			// Update the config with the initial API key
			comp.updateConfigWithAPIKey(*apiKey)
			comp.log.Info("Successfully fetched and set initial delegated API key")

			// Start the background refresh goroutine
			comp.startBackgroundRefresh()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			comp.log.Info("Stopping delegated auth background refresh...")

			// Cancel the background refresh context
			if comp.refreshCancel != nil {
				comp.refreshCancel()
			}

			comp.log.Info("Delegated auth background refresh stopped")
			return nil
		},
	})

	return comp, nil
}

// GetAPIKey returns the current API key or fetches one if it has not yet been fetched
func (d *delegatedAuthComponent) GetAPIKey(ctx context.Context) (*string, error) {
	creds, _, err := d.RefreshAndGetApiKey(ctx)
	return creds, err
}

// RefreshAPIKey fetches the API key and stores it in the component. It only returns an error if there is an issue
func (d *delegatedAuthComponent) RefreshAPIKey(ctx context.Context) error {
	_, _, err := d.RefreshAndGetApiKey(ctx)
	return err
}

// RefreshAndGetApiKey refreshes the API key and stores it in the component it returns the current API key and if a refresh actually occurred
func (d *delegatedAuthComponent) RefreshAndGetApiKey(ctx context.Context) (*string, bool, error) {
	d.mu.RLock()
	apiKey := d.apiKey
	defer d.mu.RUnlock()

	// Double-check pattern - another goroutine might have refreshed while we were waiting
	if apiKey != nil {
		return apiKey, false, nil
	}

	d.log.Info("Fetching delegated API key")

	// Authenticate with the configured provider
	apiKey, err := d.authenticate()
	if err != nil {
		d.log.Errorf("Failed to generate auth proof: %v", err)
		return nil, false, err
	}

	d.apiKey = apiKey

	return apiKey, true, nil
}

// startBackgroundRefresh starts the background goroutine that periodically refreshes the API key
func (d *delegatedAuthComponent) startBackgroundRefresh() {
	// Start background refresh
	go func() {
		ticker := time.NewTicker(d.refreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-d.refreshCtx.Done():
				// Context was canceled, exit the goroutine
				d.log.Debug("Background refresh goroutine exiting due to context cancellation")
				return
			case <-ticker.C:
				// Time to refresh
				lCreds, updated, lErr := d.RefreshAndGetApiKey(d.refreshCtx)
				if lErr != nil {
					// Check if the error is due to context cancellation
					if d.refreshCtx.Err() != nil {
						d.log.Debug("Refresh failed due to context cancellation, exiting")
						return
					}
					d.log.Errorf("Failed to refresh delegated API key: %v", lErr)
				} else {
					// Update the config with the new API key
					if updated {
						d.updateConfigWithAPIKey(*lCreds)
					}
				}
			}
		}
	}()
}

// authenticate uses the configured provider to get creds
func (d *delegatedAuthComponent) authenticate() (*string, error) {
	creds, err := d.authConfig.ProviderAuth.GetApiKey(d.config, d.authConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with AWS: %w", err)
	}
	return creds, nil
}

// Update the updateConfigWithAPIKey method to use the correct Set method
func (d *delegatedAuthComponent) updateConfigWithAPIKey(apiKey string) {
	// Update the api_key config value using the Writer interface
	// This will trigger OnUpdate callbacks for any components listening to this config
	d.config.Set("api_key", apiKey, pkgconfigmodel.SourceAgentRuntime)
	d.log.Infof("Updated config with new apiKey")
}

// noopDelegatedAuth is used when delegated auth is disabled
type noopDelegatedAuth struct{}

func (n *noopDelegatedAuth) GetAPIKey(ctx context.Context) (*string, error) {
	return nil, fmt.Errorf("delegated auth is not enabled")
}

func (n *noopDelegatedAuth) RefreshAPIKey(ctx context.Context) error {
	return fmt.Errorf("delegated auth is not enabled")
}

func (n *noopDelegatedAuth) StartApiKeyRefresh() {
	// noop
}
