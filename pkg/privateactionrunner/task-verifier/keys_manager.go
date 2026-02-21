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

type KeysManager interface {
	Start(ctx context.Context)
	GetKey(keyId string) types.DecodedKey
	WaitForReady()
}

type keysManager struct {
	rcClient               rcclient.Client
	stopChan               chan bool
	keys                   map[string]types.DecodedKey
	mu                     sync.RWMutex
	ready                  chan struct{}
	firstCallbackCompleted bool
}

func NewKeyManager(rcClient rcclient.Client) KeysManager {
	return &keysManager{
		stopChan: make(chan bool),
		keys:     make(map[string]types.DecodedKey),
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

func (k *keysManager) WaitForReady() {
	<-k.ready
}

func (k *keysManager) AgentConfigUpdateCallback(update map[string]state.RawConfig, callback func(string, state.ApplyStatus)) {
	log.Info("RC callback called")
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
