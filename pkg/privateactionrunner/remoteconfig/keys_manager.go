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

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type KeysManager interface {
	Start(ctx context.Context)
	GetKey(keyId string) types.DecodedKey
	WaitForReady()
}

// TODO Copied from comp/remote-config/rcclient/component.go. Import it ? but I don't think we want to depend on `comp` things from `pkg`
type RcClient interface {
	Subscribe(product data.Product, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
}

type keysManager struct {
	stopChan               chan bool
	keys                   map[string]types.DecodedKey
	mu                     sync.RWMutex
	ready                  chan struct{}
	firstCallbackCompleted bool
	rcClient               RcClient
}

func New(rcClient RcClient) KeysManager {
	return &keysManager{
		stopChan: make(chan bool),
		keys:     make(map[string]types.DecodedKey),
		ready:    make(chan struct{}),
		rcClient: rcClient,
	}
}

func (k *keysManager) Start(ctx context.Context) {
	log.Info("Subscribing to remote config updates")
	k.rcClient.Subscribe(state.ProductActionPlatformRunnerKeys, k.AgentConfigUpdateCallback)
}

func (k *keysManager) GetKey(keyId string) types.DecodedKey {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.keys[keyId]
}

func (k *keysManager) WaitForReady() {
	//<-k.ready // TODO fix this
	return
}

func (k *keysManager) AgentConfigUpdateCallback(update map[string]state.RawConfig, callback func(string, state.ApplyStatus)) {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.keys = make(map[string]types.DecodedKey) // clear the current keys
	for configId, rawConfig := range update {
		decodedKey, err := k.decode(rawConfig)
		if err != nil {
			log.Errorf("Failed to decode remote config %v")
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
	log.Info("Successfully updated keys", k.keys)
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

	log.Infof("decoding key %s of type %s", rawConfig.Metadata.ID, rawKey.KeyType)
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
