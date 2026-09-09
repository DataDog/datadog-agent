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
	"sync"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// UpdateCallback receives signing-key updates from Remote Config.
type UpdateCallback func(map[string]state.RawConfig, func(string, state.ApplyStatus))

type keysManager struct {
	rcClient               rcclient.Client
	stopChan               chan bool
	keys                   map[string]storedKey
	mu                     sync.RWMutex
	ready                  chan struct{}
	firstCallbackCompleted bool
}

type storedKey struct {
	key        types.DecodedKey
	targetPath string
}

// NewKeyManager returns a key manager backed by Remote Config.
func NewKeyManager(rcClient rcclient.Client) KeysManager {
	manager, _ := NewKeyManagerWithCallback()
	if manager, ok := manager.(*keysManager); ok {
		manager.rcClient = rcClient
	}
	return manager
}

// NewKeyManagerWithCallback returns a key manager and the callback to register
// with Remote Config before its polling loop starts.
func NewKeyManagerWithCallback() (KeysManager, UpdateCallback) {
	manager := &keysManager{
		stopChan: make(chan bool),
		keys:     make(map[string]storedKey),
		ready:    make(chan struct{}),
	}
	return manager, manager.AgentConfigUpdateCallback
}

func (k *keysManager) Start(ctx context.Context) {
	if k.rcClient != nil {
		log.FromContext(ctx).Info("Subscribing to remote config updates")
		k.rcClient.Subscribe(state.ProductActionPlatformRunnerKeys, k.AgentConfigUpdateCallback)
	}
}

func (k *keysManager) GetKey(keyId string) (types.DecodedKey, *types.DirectorKeyProof) {
	k.mu.RLock()
	entry, ok := k.keys[keyId]
	k.mu.RUnlock()
	if !ok {
		return nil, nil
	}
	if k.rcClient == nil {
		return entry.key, nil
	}
	proof, ok := k.rcClient.GetConfigTUFProof(entry.targetPath)
	if !ok {
		return entry.key, nil
	}
	return entry.key, &types.DirectorKeyProof{
		Roots:      proof.Roots,
		Targets:    proof.Targets,
		TargetPath: proof.TargetPath,
		TargetFile: proof.TargetFile,
	}
}

func (k *keysManager) WaitForReady() {
	<-k.ready
}

func (k *keysManager) AgentConfigUpdateCallback(update map[string]state.RawConfig, callback func(string, state.ApplyStatus)) {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.keys = make(map[string]storedKey) // clear the current keys
	for configPath, rawConfig := range update {
		decodedKey, err := decode(rawConfig)
		if err != nil {
			log.Error("Failed to decode remote config", log.ErrorField(err))
			callback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}
		k.keys[rawConfig.Metadata.ID] = storedKey{key: decodedKey, targetPath: configPath}
		callback(configPath, state.ApplyStatus{
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
