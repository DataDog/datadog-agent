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
	provider        delegatedauthpkg.DelegatedAuthProvider
	authConfig      *delegatedauthpkg.DelegatedAuthConfig
	refreshInterval time.Duration
}

type dependencies struct {
	fx.In
	Config config.Component
	Log    log.Component
	Lc     fx.Lifecycle
}

func NewDelegatedAuth(deps dependencies) (delegatedauth.Component, error) {
	if !deps.Config.GetBool("delegated_auth.enabled") {
		return &noopDelegatedAuth{}, nil
	}

	provider := deps.Config.GetString("delegated_auth.provider")
	orgUUID := deps.Config.GetString("delegated_auth.org_uuid")
	refreshInterval := deps.Config.GetDuration("delegated_auth.refresh_interval") * time.Second

	if refreshInterval == 0 {
		// Default to 1 second if no refresh interval
		refreshInterval = 1 * time.Second
	}

	if orgUUID == "" {
		return nil, fmt.Errorf("delegated_auth.org_uuid is required when delegated_auth.enabled is true")
	}

	var tokenProvider delegatedauthpkg.DelegatedAuthProvider
	switch provider {
	case cloudauth.ProviderAWS:
		tokenProvider = &cloudauth.AWSAuth{
			AwsRegion: deps.Config.GetString("delegated_auth.aws_region"),
		}
	default:
		return nil, fmt.Errorf("unsupported delegated auth provider: %s", provider)
	}

	authConfig := &delegatedauthpkg.DelegatedAuthConfig{
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

			// Fetch the initial API key synchronously during startup
			apiKey, err := comp.GetApiKey(ctx)
			if err != nil {
				comp.log.Errorf("Failed to get initial delegated API key: %v", err)
				// Return nil here to not stop the agent from starting
				return nil
			}

			// Update the config with the initial API key
			comp.updateConfigWithApiKey(*apiKey)
			comp.log.Info("Successfully fetched and set initial delegated API key")

			// Start the background refresh goroutine
			comp.startBackgroundRefresh()

			return nil
		},
	})

	return comp, nil
}

func (d *delegatedAuthComponent) GetApiKey(ctx context.Context) (*string, error) {
	d.mu.RLock()
	if d.apiKey != nil {
		apiKey := d.apiKey
		d.mu.RUnlock()
		return apiKey, nil
	}
	d.mu.RUnlock()

	creds, _, err := d.RefreshAndGetApiKey(ctx)
	return creds, err
}

func (d *delegatedAuthComponent) RefreshApiKey(ctx context.Context) error {
	_, _, err := d.RefreshAndGetApiKey(ctx)
	return err
}

func (d *delegatedAuthComponent) RefreshAndGetApiKey(ctx context.Context) (*string, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Double-check pattern - another goroutine might have refreshed while we were waiting
	if d.apiKey != nil {
		return d.apiKey, false, nil
	}

	d.log.Info("Refreshing delegated API key")

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
	if d.refreshInterval < 0 {
		d.refreshInterval = 1 * time.Second
	}
	d.log.Infof("Setting refresh interval to %s", d.refreshInterval)

	// Start background refresh
	go func() {
		ticker := time.NewTicker(d.refreshInterval)
		defer ticker.Stop()

		ctx := context.Background()
		for range ticker.C {
			lCreds, updated, lErr := d.RefreshAndGetApiKey(ctx)
			if lErr != nil {
				d.log.Errorf("Failed to refresh delegated API key: %v", lErr)
			} else {
				// Update the config with the new API key
				if updated {
					d.updateConfigWithApiKey(*lCreds)
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

// Update the updateConfigWithApiKey method to use the correct Set method
func (d *delegatedAuthComponent) updateConfigWithApiKey(apiKey string) {
	// Update the api_key config value using the Writer interface
	// This will trigger OnUpdate callbacks for any components listening to this config
	d.config.Set("api_key", apiKey, pkgconfigmodel.SourceAgentRuntime)
	d.log.Infof("Updated config with new apiKey")
}

// noopDelegatedAuth is used when delegated auth is disabled
type noopDelegatedAuth struct{}

func (n *noopDelegatedAuth) GetApiKey(ctx context.Context) (*string, error) {
	return nil, fmt.Errorf("delegated auth is not enabled")
}

func (n *noopDelegatedAuth) RefreshApiKey(ctx context.Context) error {
	return fmt.Errorf("delegated auth is not enabled")
}

func (n *noopDelegatedAuth) StartApiKeyRefresh() {
	// noop
}
