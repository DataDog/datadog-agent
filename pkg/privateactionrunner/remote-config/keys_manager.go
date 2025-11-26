// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remoteconfig

import (
	"context"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	rcservice "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	configsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/viperconfig"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

type KeysManager interface {
	Start(ctx context.Context)
	Close(ctx context.Context)
	GetKey(keyId string) types.DecodedKey
	WaitForReady()
}

type keysManager struct {
	rcService              *rcservice.CoreAgentService
	rcClient               *rcclient.Client
	stopChan               chan bool
	config                 *config.Config
	keys                   map[string]types.DecodedKey
	mu                     sync.RWMutex
	ready                  chan struct{}
	firstCallbackCompleted bool
}

type dummyTelemetryReporter struct{}

func (d dummyTelemetryReporter) IncRateLimit()                              {}
func (d dummyTelemetryReporter) IncTimeout()                                {}
func (d dummyTelemetryReporter) IncConfigSubscriptionsConnectedCounter()    {}
func (d dummyTelemetryReporter) IncConfigSubscriptionsDisconnectedCounter() {}
func (d dummyTelemetryReporter) SetConfigSubscriptionsActive(_ int)         {}
func (d dummyTelemetryReporter) SetConfigSubscriptionClientsTracked(_ int)  {}

func New(ctx context.Context, config *config.Config) (KeysManager, error) {
	// provide a config object to the remote config client
	cfg := viperconfig.NewViperConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	// This loads HTTP_PROXY, HTTPS_PROXY and NO_PROXY environment variables needed for the remote config client to communicate
	// through proxies, as the client's transport is configured using these settings.
	// See https://github.com/DataDog/datadog-agent/blob/f376a75844c46d97c580b25151ccff815e9eabf1/pkg/util/http/transport.go#L125
	configsetup.LoadProxyFromEnv(cfg)
	hostname, _ := os.Hostname()
	remoteConfigEndpoint := strings.Join([]string{"https://config.", config.DatadogSite}, "")
	startPARJWT, err := util.GeneratePARJWT(config.OrgId, config.RunnerId, config.PrivateKey, nil)
	if err != nil {
		return nil, err
	}

	rcService, err := rcservice.NewService(
		cfg,
		"",
		remoteConfigEndpoint,
		hostname,
		func() []string {
			return []string{
				fmt.Sprintf("site:%s", config.DatadogSite),
				fmt.Sprintf("runner-id:%s", config.RunnerId),
			}
		},
		&dummyTelemetryReporter{},
		// TODO fix when other values accepted
		"7.50.0",
		// TODO revert back to instantiation WithPARJWT(), when the headers bug is fixed (DD-PAR-JWT vs Dd-Par-Jwt)
		// rcservice.WithPARJWT(startPARJWT),
		rcservice.WithDatabaseFileName(filepath.Join(os.TempDir(), "dd-private-action-runner", fmt.Sprintf("remote-config-%s.db", config.RunnerId))),
		rcservice.WithConfigRootOverride(config.DatadogSite, ""),
		rcservice.WithDirectorRootOverride(config.DatadogSite, ""),
	)
	if err != nil {
		log.FromContext(ctx).Error("Failed to create Remote Configuration service", log.ErrorField(err))
		return nil, err
	}
	rcService.UpdatePARJWT(startPARJWT)

	rcClient, err := rcclient.NewClient(
		rcService,
		rcclient.WithAgent("private-action-runner", config.Version),
		rcclient.WithProducts(state.ProductActionPlatformRunnerKeys),
		rcclient.WithDirectorRootOverride(config.DatadogSite, ""),
	)
	if err != nil {
		return nil, err
	}
	return &keysManager{
		rcService: rcService,
		rcClient:  rcClient,
		config:    config,
		stopChan:  make(chan bool),
		keys:      make(map[string]types.DecodedKey),
		ready:     make(chan struct{}),
	}, nil
}

func (k *keysManager) Start(ctx context.Context) {
	log.FromContext(ctx).Info("Subscribing to remote config updates")
	k.rcClient.Subscribe(state.ProductActionPlatformRunnerKeys, k.AgentConfigUpdateCallback)
	log.FromContext(ctx).Info("Starting remote config service")
	k.rcService.Start()
	// there is an issue in RC when the client and the service start at the same time it can make them miss the first update.
	// the next update will be after 50 seconds and we need to have the keys before starting to poll for tasks so it blocks the entire startup.
	// Sleeping for 2 seconds actually makes us start faster in the majority of cases.
	// this can be removed once we upgrade to the rc version with the following fix
	// https://github.com/DataDog/datadog-agent/pull/34844
	time.Sleep(2 * time.Second)
	log.FromContext(ctx).Info("Starting remote config client")
	k.rcClient.Start()
	log.FromContext(ctx).Infof("Starting PAR JWT rotation job with interval %f seconds", k.config.JWTRefreshInterval.Seconds())
	k.jobPARJWTRotation(ctx, k.config.JWTRefreshInterval)
}

func (k *keysManager) Close(ctx context.Context) {
	log.FromContext(ctx).Info("Stopping remote config service")
	err := k.rcService.Stop()
	if err != nil {
		log.FromContext(ctx).Error("Failed to stop remote config service", log.ErrorField(err))
	}
	log.FromContext(ctx).Info("Stopping PAR JWT rotation job")
	k.stopChan <- true
	log.FromContext(ctx).Info("Stopping remote config client")
	k.rcClient.Close()
}

func (k *keysManager) GetKey(keyId string) types.DecodedKey {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.keys[keyId]
}

func (k *keysManager) WaitForReady() {
	<-k.ready
}

func (k *keysManager) AgentConfigUpdateCallback(update map[string]state.RawConfig, callback func(string, state.ApplyStatus)) {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.keys = make(map[string]types.DecodedKey) // clear the current keys
	for configId, rawConfig := range update {
		decodedKey, err := decode(rawConfig)
		if err != nil {
			log.Error("Failed to decode remote config", log.ErrorField(err))
			callback(configId, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}
		k.keys[rawConfig.Metadata.ID] = decodedKey
		callback(configId, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})
	}
	log.Info("Successfully updated keys", log.Any("keys", k.keys))
	if !k.firstCallbackCompleted {
		k.firstCallbackCompleted = true
		close(k.ready)
	}
}

func decode(rawConfig state.RawConfig) (types.DecodedKey, error) {
	k := types.RawKey{}
	err := json.Unmarshal(rawConfig.Config, &k)
	if err != nil {
		return nil, fmt.Errorf("json decoding error: %w", err)
	}

	log.Infof("decoding key %s of type %s", rawConfig.Metadata.ID, k.KeyType)
	switch k.KeyType {
	case types.KeyTypeX509RSA:
		return decodeX509RSA(k)
	case types.KeyTypeED25519:
		return decodeED25519(k)
	default:
		return nil, fmt.Errorf("unsupported key type: %s", k.KeyType)
	}
}

func decodeX509RSA(k types.RawKey) (*types.X509RSAKey, error) {
	blocks, _ := pem.Decode(k.Key)
	if blocks == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	cert, err := x509.ParseCertificate(blocks.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}
	return &types.X509RSAKey{
		KeyType: k.KeyType,
		Key:     cert.PublicKey.(*rsa.PublicKey),
	}, nil
}

func decodeED25519(k types.RawKey) (*types.ED25519Key, error) {
	blocks, _ := pem.Decode(k.Key)
	if blocks == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	keyAny, err := x509.ParsePKIXPublicKey(blocks.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ED25519 public key: %w", err)
	}
	keyED25519, ok := keyAny.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("failed to cast to ed25519.PublicKey")
	}
	return &types.ED25519Key{
		KeyType: k.KeyType,
		Key:     keyED25519,
	}, nil
}

func (k *keysManager) getPARJWT() (string, error) {
	return util.GeneratePARJWT(k.config.OrgId, k.config.RunnerId, k.config.PrivateKey, nil)
}

func (k *keysManager) jobPARJWTRotation(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				newPARJWT, err := k.getPARJWT()
				if err != nil {
					log.FromContext(ctx).Error("Failed to update PAR JWT", log.ErrorField(err))
					continue
				} else {
					k.rcService.UpdatePARJWT(newPARJWT)
					log.FromContext(ctx).Debug("Successfully updated PAR JWT")
				}
			case <-k.stopChan:
				return
			}
		}
	}()
}
