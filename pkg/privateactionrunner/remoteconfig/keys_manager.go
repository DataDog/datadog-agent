package remoteconfig

import (
	"context"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	rcservice "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/helpers"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
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
	log                    log.Component
}

type dummyTelemetryReporter struct{}

func (d dummyTelemetryReporter) IncRateLimit() {}
func (d dummyTelemetryReporter) IncTimeout()   {}

func New(ctx context.Context, config *config.Config, rcService *rcservice.CoreAgentService) (KeysManager, error) {
	// provide a config object to the remote config client
	startPARJWT, err := helpers.GeneratePARJWT(config.OrgId, config.RunnerId, config.PrivateKey, nil)
	if err != nil {
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
	k.log.Info("Subscribing to remote config updates")
	k.rcClient.Subscribe(state.ProductActionPlatformRunnerKeys, k.AgentConfigUpdateCallback)
	k.log.Info("Starting remote config service")
	k.rcService.Start()
	// there is an issue in RC when the client and the service start at the same time it can make them miss the first update.
	// the next update will be after 50 seconds and we need to have the keys before starting to poll for tasks so it blocks the entire startup.
	// Sleeping for 2 seconds actually makes us start faster in the majority of cases.
	// this can be removed once we upgrade to the rc version with the following fix
	// https://github.com/DataDog/datadog-agent/pull/34844
	time.Sleep(2 * time.Second)
	k.log.Info("Starting remote config client")
	k.rcClient.Start()
	k.log.Infof("Starting PAR JWT rotation job with interval %f seconds", k.config.JWTRefreshInterval.Seconds())
	k.jobPARJWTRotation(ctx, k.config.JWTRefreshInterval)
}

func (k *keysManager) Close(ctx context.Context) {
	k.log.Info("Stopping remote config service")
	err := k.rcService.Stop()
	if err != nil {
		k.log.Errorf("Failed to stop remote config service %v", err)
	}
	k.log.Info("Stopping PAR JWT rotation job")
	k.stopChan <- true
	k.log.Info("Stopping remote config client")
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
		decodedKey, err := k.decode(rawConfig)
		if err != nil {
			k.log.Errorf("Failed to decode remote config %v")
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
	k.log.Info("Successfully updated keys", k.keys)
	if !k.firstCallbackCompleted {
		k.firstCallbackCompleted = true
		close(k.ready)
	}
}

func (k *keysManager) decode(rawConfig state.RawConfig) (types.DecodedKey, error) {
	var rawKey types.RawKey
	err := json.Unmarshal(rawConfig.Config, &rawKey)
	if err != nil {
		return nil, fmt.Errorf("json decoding error: %w", err)
	}

	k.log.Infof("decoding key %s of type %s", rawConfig.Metadata.ID, rawKey.KeyType)
	switch rawKey.KeyType {
	case types.KeyTypeX509RSA:
		return decodeX509RSA(rawKey)
	case types.KeyTypeED25519:
		return decodeED25519(rawKey)
	default:
		return nil, fmt.Errorf("unsupported key type: %s", rawKey.KeyType)
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
	return helpers.GeneratePARJWT(k.config.OrgId, k.config.RunnerId, k.config.PrivateKey, nil)
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
					k.log.Errorf("Failed to update PAR JWT %v", err)
					continue
				} else {
					k.rcService.UpdatePARJWT(newPARJWT)
					k.log.Debug("Successfully updated PAR JWT")
				}
			case <-k.stopChan:
				return
			}
		}
	}()
}
