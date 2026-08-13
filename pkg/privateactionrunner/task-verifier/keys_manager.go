// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package taskverifier

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/signingkeys"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type keysManager struct {
	rcClient rcclient.Client

	mu           sync.RWMutex
	keys         map[string]types.DecodedKey
	ready        bool
	readyChanged chan struct{}
}

type noOpKeysManager struct{}

func (n *noOpKeysManager) Start(context.Context)                   {}
func (n *noOpKeysManager) GetKey(string) types.DecodedKey          { return nil }
func (n *noOpKeysManager) WaitForReady(context.Context) error      { return nil }
func (n *noOpKeysManager) InstallAuthoritative([]SigningKey) error { return nil }
func (n *noOpKeysManager) MarkExpired()                            {}
func (n *noOpKeysManager) IsReady() bool                           { return true }

// NewKeyManager returns a manager backed by the monolithic runner's Remote Config client.
func NewKeyManager(rcClient rcclient.Client) KeysManager {
	return newKeysManager(rcClient)
}

// NewExecutorKeyManager returns a manager populated by the Core Agent signing-key API.
func NewExecutorKeyManager() KeysManager {
	return newKeysManager(nil)
}

func newKeysManager(rcClient rcclient.Client) KeysManager {
	if os.Getenv(app.InternalSkipTaskVerificationEnvVar) == "true" {
		return &noOpKeysManager{}
	}
	return &keysManager{
		rcClient:     rcClient,
		keys:         make(map[string]types.DecodedKey),
		readyChanged: make(chan struct{}),
	}
}

func (k *keysManager) Start(ctx context.Context) {
	if k.rcClient == nil {
		return
	}
	log.FromContext(ctx).Info("Subscribing to remote config updates")
	k.rcClient.Subscribe(state.ProductActionPlatformRunnerKeys, k.AgentConfigUpdateCallback)
}

func (k *keysManager) GetKey(keyID string) types.DecodedKey {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.keys[keyID]
}

func (k *keysManager) WaitForReady(ctx context.Context) error {
	for {
		k.mu.RLock()
		if k.ready {
			k.mu.RUnlock()
			return nil
		}
		changed := k.readyChanged
		k.mu.RUnlock()

		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (k *keysManager) IsReady() bool {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.ready
}

// InstallAuthoritative validates a complete snapshot before replacing the current keys.
// An empty snapshot is authoritative and establishes readiness.
func (k *keysManager) InstallAuthoritative(keys []SigningKey) error {
	decoded := make(map[string]types.DecodedKey, len(keys))
	for _, key := range keys {
		if key.ID == "" {
			return errors.New("signing key ID is empty")
		}
		if _, duplicate := decoded[key.ID]; duplicate {
			return fmt.Errorf("duplicate signing key ID %q", key.ID)
		}
		decodedKey, err := signingkeys.DecodePublicKey(key)
		if err != nil {
			return fmt.Errorf("decoding signing key %q: %w", key.ID, err)
		}
		decoded[key.ID] = decodedKey
	}

	k.mu.Lock()
	defer k.mu.Unlock()
	k.keys = decoded
	k.setReadyLocked(true)
	return nil
}

// MarkExpired makes the manager reject new work while preserving installed keys.
func (k *keysManager) MarkExpired() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.setReadyLocked(false)
}

func (k *keysManager) setReadyLocked(ready bool) {
	if k.ready == ready {
		return
	}
	k.ready = ready
	close(k.readyChanged)
	k.readyChanged = make(chan struct{})
}

func (k *keysManager) AgentConfigUpdateCallback(update map[string]state.RawConfig, callback func(string, state.ApplyStatus)) {
	keys, decodeErrs := signingkeys.DecodeSnapshot(update)
	if len(decodeErrs) != 0 {
		for configID := range update {
			err := decodeErrs[configID]
			if err == nil {
				err = errors.New("complete signing-key snapshot rejected")
			}
			callback(configID, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
		}
		return
	}
	if err := k.InstallAuthoritative(keys); err != nil {
		log.Error("Failed to install remote config signing keys", log.ErrorField(err))
		return
	}
	for configID := range update {
		callback(configID, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
	log.Info("Successfully updated signing keys")
}
