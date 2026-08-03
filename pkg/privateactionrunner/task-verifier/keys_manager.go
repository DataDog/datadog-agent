// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package taskverifier

import (
	"context"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"

	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type keysManager struct {
	rcClient rcclient.Client
	stopChan chan bool

	mu        sync.RWMutex
	keys      map[string]types.DecodedKey
	rawKeys   map[string]SigningKey
	ready     chan struct{}
	readyOnce sync.Once
	rcReady   bool
	seeded    bool
}

// noOpKeysManager satisfies KeysManager without requiring Remote Config.
// WaitForReady returns immediately. Used when DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true.
type noOpKeysManager struct{}

func (n *noOpKeysManager) Start(_ context.Context)          {}
func (n *noOpKeysManager) GetKey(_ string) types.DecodedKey { return nil }
func (n *noOpKeysManager) WaitForReady(_ context.Context) error {
	return nil
}
func (n *noOpKeysManager) Seed(_ []SigningKey) error { return nil }
func (n *noOpKeysManager) Snapshot() []SigningKey    { return nil }

// NewKeyManager returns a KeysManager appropriate for the current environment.
// When DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true, a no-op manager is returned.
func NewKeyManager(rcClient rcclient.Client) KeysManager {
	if os.Getenv(app.InternalSkipTaskVerificationEnvVar) == "true" {
		return &noOpKeysManager{}
	}
	return &keysManager{
		stopChan: make(chan bool),
		keys:     make(map[string]types.DecodedKey),
		rawKeys:  make(map[string]SigningKey),
		ready:    make(chan struct{}),
		rcClient: rcClient,
	}
}

func (k *keysManager) Start(ctx context.Context) {
	log.FromContext(ctx).Info("Subscribing to remote config updates")
	k.rcClient.Subscribe(state.ProductActionPlatformRunnerKeys, k.AgentConfigUpdateCallback)
}

func (k *keysManager) GetKey(keyId string) types.DecodedKey {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.keys[keyId]
}

func (k *keysManager) WaitForReady(ctx context.Context) error {
	select {
	case <-k.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Seed installs a cached key snapshot only while this executor is cold. A
// verified Remote Config callback always wins and replaces the seeded snapshot.
// The seed deliberately does not satisfy WaitForReady: only a callback from the
// current executor's Remote Config client proves that the snapshot is fresh.
func (k *keysManager) Seed(keys []SigningKey) error {
	if len(keys) == 0 {
		return nil
	}

	decoded := make(map[string]types.DecodedKey, len(keys))
	raw := make(map[string]SigningKey, len(keys))
	for _, signingKey := range keys {
		if signingKey.ID == "" {
			return errors.New("signing key ID is empty")
		}
		decodedKey, err := decodeSigningKey(signingKey)
		if err != nil {
			return fmt.Errorf("decoding seeded key %q: %w", signingKey.ID, err)
		}
		decoded[signingKey.ID] = decodedKey
		raw[signingKey.ID] = cloneSigningKey(signingKey)
	}

	k.mu.Lock()
	defer k.mu.Unlock()
	if k.rcReady || k.seeded {
		return nil
	}
	k.keys = decoded
	k.rawKeys = raw
	k.seeded = true
	return nil
}

// Snapshot returns a stable copy suitable for transfer to the control plane.
func (k *keysManager) Snapshot() []SigningKey {
	k.mu.RLock()
	defer k.mu.RUnlock()

	keys := make([]SigningKey, 0, len(k.rawKeys))
	for _, signingKey := range k.rawKeys {
		keys = append(keys, cloneSigningKey(signingKey))
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].ID < keys[j].ID })
	return keys
}

func (k *keysManager) AgentConfigUpdateCallback(update map[string]state.RawConfig, callback func(string, state.ApplyStatus)) {
	k.mu.Lock()
	defer k.mu.Unlock()

	decodedKeys := make(map[string]types.DecodedKey, len(update))
	rawKeys := make(map[string]SigningKey, len(update))
	for configID, rawConfig := range update {
		decodedKey, signingKey, err := decode(rawConfig)
		if err != nil {
			log.Error("Failed to decode remote config", log.ErrorField(err))
			callback(configID, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}
		decodedKeys[signingKey.ID] = decodedKey
		rawKeys[signingKey.ID] = signingKey
		callback(configID, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})
	}
	k.keys = decodedKeys
	k.rawKeys = rawKeys
	k.rcReady = true
	log.Info("Successfully updated keys", log.Any("keys", k.keys))
	k.readyOnce.Do(func() { close(k.ready) })
}

func decode(rawConfig state.RawConfig) (types.DecodedKey, SigningKey, error) {
	rawKey := types.RawKey{}
	if err := json.Unmarshal(rawConfig.Config, &rawKey); err != nil {
		return nil, SigningKey{}, fmt.Errorf("json decoding error: %w", err)
	}

	signingKey := SigningKey{
		ID:      rawConfig.Metadata.ID,
		KeyType: rawKey.KeyType,
		Key:     append([]byte(nil), rawKey.Key...),
	}
	decodedKey, err := decodeSigningKey(signingKey)
	return decodedKey, signingKey, err
}

func decodeSigningKey(signingKey SigningKey) (types.DecodedKey, error) {
	log.Infof("decoding key %s of type %s", signingKey.ID, signingKey.KeyType)
	rawKey := types.RawKey{KeyType: signingKey.KeyType, Key: signingKey.Key}
	switch rawKey.KeyType {
	case types.KeyTypeX509RSA:
		return decodeX509RSA(rawKey)
	case types.KeyTypeED25519:
		return decodeED25519(rawKey)
	default:
		return nil, fmt.Errorf("unsupported key type: %s", rawKey.KeyType)
	}
}

func cloneSigningKey(key SigningKey) SigningKey {
	key.Key = append([]byte(nil), key.Key...)
	return key
}

func decodeX509RSA(k types.RawKey) (*types.X509RSAKey, error) {
	blocks, _ := pem.Decode(k.Key)
	if blocks == nil {
		return nil, errors.New("failed to decode PEM block")
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
		return nil, errors.New("failed to decode PEM block")
	}
	keyAny, err := x509.ParsePKIXPublicKey(blocks.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ED25519 public key: %w", err)
	}
	keyED25519, ok := keyAny.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("failed to cast to ed25519.PublicKey")
	}
	return &types.ED25519Key{
		KeyType: k.KeyType,
		Key:     keyED25519,
	}, nil
}
