// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ncm

// Package store is provides persistent local storage for network device configurations (for NCM)
// utilizing bbolt - enabling rollback of configs w/o sending data to the Datadog backend
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// Bucket names for bbolt
	rawConfigBucket    = "raw_config" // TODO: temporary bucket, a hybrid approach until the blocks logic is ironed out
	configBlocksBucket = "config_blocks"
	metadataBucket     = "metadata"
	secretsBucket      = "secrets"

	// DB configurations
	ownerRWFileMode     = 0600 // only the owner can read/write
	databaseLockTimeout = 1 * time.Second
)

// ConfigStore implements persistent KV store for configurations for rollbacks
// whenever a config is retrieved, we will store agent-side along with the payload sent
// to intake to enable "rollbacks" without sending sensitive data (in configs) back and forth
type ConfigStore struct {
	db   *bbolt.DB
	lock sync.RWMutex
}

// Open creates a new ConfigStore and initializes the underlying boltDB + required buckets
func Open(path string) (*ConfigStore, error) {
	db, err := bbolt.Open(path, ownerRWFileMode, &bbolt.Options{
		Timeout: databaseLockTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open NCM bbolt config store at %s: %w", path, err)
	}

	cs := &ConfigStore{db: db}

	// Create the buckets when we first open ?
	err = cs.update(func(tx *bbolt.Tx) error {
		for _, name := range []string{rawConfigBucket, configBlocksBucket, metadataBucket, secretsBucket} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return fmt.Errorf("error creating bucket %s: %w", name, err)
			}
		}
		return nil
	})
	if err != nil {
		cs.Close()
		return nil, err
	}

	return cs, nil
}

// Close cleans up / releases DB resources
func (cs *ConfigStore) Close() error {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	return cs.db.Close()
}

// Base helper transaction functions for the DB

// view wraps the bbolt View transaction with a read lock (for ease of use)
func (cs *ConfigStore) view(fn func(tx *bbolt.Tx) error) error {
	cs.lock.RLock()
	defer cs.lock.RUnlock()
	return cs.db.View(fn)
}

// update wraps the bbolt Update transaction with a write lock (for ease of use)
func (cs *ConfigStore) update(fn func(tx *bbolt.Tx) error) error {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	return cs.db.Update(fn)
}

// NCM-specific transaction functions

// StoreConfig is responsible for checking if the config for the device is new,
// if so, it will create a new entry in each bucket (for the config, metadata, and secrets)
func (cs *ConfigStore) StoreConfig(deviceID, configType string, rawConfig string, blocks []ConfigBlock, secrets map[string]string) (string, error) {
	// Check that this is a new config for the DB - does the hash match the last stored config for this device?
	// TODO: implement the above functionality in a separate method
	// for consideration: utilizing a composite key that is made up of
	// config_type | device_id | timestamp | uuid (or using this with another bucket to emulate an index)

	// Setup for storing the config
	configUUID := uuid.New().String()
	now := time.Now().Unix() // TODO: may need to be different for testing purposes

	// Raw text
	rawConfigJSON, err := json.Marshal(rawConfig)
	if err != nil {
		return "", fmt.Errorf("marshal raw config error: %w", err)
	}

	// Blocks / raw text
	blocksJSON, err := json.Marshal(blocks)
	if err != nil {
		return "", fmt.Errorf("marshal config blocks error: %w", err)
	}

	// Metadata
	metadata := ConfigMetadata{
		ConfigUUID:     configUUID,
		DeviceID:       deviceID,
		ConfigType:     configType,
		CapturedAt:     now,
		LastAccessedAt: now,
		RawHash:        hashConfig(rawConfig),
		AgentVersion:   version.AgentVersion,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("marshal config metadata error: %w", err)
	}

	// Secrets
	secretsJSON, err := json.Marshal(secrets)
	if err != nil {
		return "", fmt.Errorf("marshal secrets error: %w", err)
	}

	// Update the DB with all the JSONs
	err = cs.update(func(tx *bbolt.Tx) error {
		key := []byte(configUUID) // TODO: include more for prefix searches?
		if err := tx.Bucket([]byte(rawConfigBucket)).Put(key, rawConfigJSON); err != nil {
			return err
		}
		if err := tx.Bucket([]byte(configBlocksBucket)).Put(key, blocksJSON); err != nil {
			return err
		}
		if err := tx.Bucket([]byte(metadataBucket)).Put(key, metadataJSON); err != nil {
			return err
		}
		if err := tx.Bucket([]byte(secretsBucket)).Put(key, secretsJSON); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("error storing config in bbolt: %w", err)
	}

	return configUUID, nil
}

// GetConfig retrieves all the data associated with a config given its UUID
func (cs *ConfigStore) GetConfig(configUUID string) (string, []ConfigBlock, *ConfigMetadata, map[string]string, error) {
	var rawConfig string
	var blocks []ConfigBlock
	var metadata ConfigMetadata
	var secrets map[string]string

	err := cs.view(func(tx *bbolt.Tx) error {
		key := []byte(configUUID) // TODO: keep UUID as key vs. composite key / index?

		// Unmarshal raw config
		rawConfigBytes := tx.Bucket([]byte(rawConfigBucket)).Get(key)
		if rawConfigBytes == nil {
			return fmt.Errorf("raw config not found for UUID: %s", configUUID)
		}
		if err := json.Unmarshal(rawConfigBytes, &rawConfig); err != nil {
			return fmt.Errorf("unmarshal raw config error: %w", err)
		}

		// Unmarshal blocks
		blocksBytes := tx.Bucket([]byte(configBlocksBucket)).Get(key)
		if blocksBytes == nil {
			return fmt.Errorf("blocks not found for UUID: %s", configUUID)
		}
		if err := json.Unmarshal(blocksBytes, &blocks); err != nil {
			return fmt.Errorf("unmarshal blocks error: %w", err)
		}

		// Unmarshal metadata
		metadataBytes := tx.Bucket([]byte(metadataBucket)).Get(key)
		if metadataBytes == nil {
			return fmt.Errorf("metadata not found for UUID: %s", configUUID)
		}
		if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
			return fmt.Errorf("unmarshal metadata error: %w", err)
		}
		// Unmarshal secrets
		secretBytes := tx.Bucket([]byte(secretsBucket)).Get(key)
		if secretBytes == nil {
			return fmt.Errorf("secrets not found for UUID: %s", configUUID)
		}
		if err := json.Unmarshal(secretBytes, &secrets); err != nil {
			return fmt.Errorf("unmarshal secrets error: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", nil, nil, nil, err
	}

	return rawConfig, blocks, &metadata, secrets, nil
}

// hashConfig returns a SHA-256 hash of the config content as a string
func hashConfig(raw string) string {
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}

// buildEvictionIndex scans the metadata bucket and returns:
// configsPerDevice: map of deviceID -> total number of configs stored for that device
// sortedEntries: all ConfigMetadata pointers sorted by LastAccessedAt ascending (oldest first)
// Both structures are built in a single view transaction for a consistent snapshot.
func (cs *ConfigStore) buildEvictionIndex() (configsPerDevice map[string]int, entries []*ConfigMetadata, err error) {
	configsPerDevice = make(map[string]int)

	err = cs.view(func(tx *bbolt.Tx) error {
		return tx.Bucket([]byte(metadataBucket)).ForEach(func(_, v []byte) error {
			var meta ConfigMetadata
			if err := json.Unmarshal(v, &meta); err != nil {
				return fmt.Errorf("unmarshal metadata error during eviction index build: %w", err)
			}
			configsPerDevice[meta.DeviceID]++
			entries = append(entries, &meta)
			return nil
		})
	})
	if err != nil {
		return nil, nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].LastAccessedAt < entries[j].LastAccessedAt
	})

	return configsPerDevice, entries, nil
}

// getEvictableExceedingN returns UUIDs of configs to evict due to the per-device cap N
// For each device whose total config count exceeds N, evict the oldest evictable configs
// This function should be called first before calling for the global LRU candidate
func getEvictableExceedingMax(configsPerDevice map[string]int, sortedEntries []*ConfigMetadata, maxRetainedConfigs int) ([]string, []*ConfigMetadata) {
	var evictable []string
	var remaining []*ConfigMetadata

	for _, entry := range sortedEntries {
		if entry.IsPinned || configsPerDevice[entry.DeviceID] <= maxRetainedConfigs {
			remaining = append(remaining, entry)
			continue
		}
		evictable = append(evictable, entry.ConfigUUID)
		configsPerDevice[entry.DeviceID]--
	}
	return evictable, remaining
}

// getGlobalLRUCandidate returns the UUID of the single oldest evictable config (rule 3).
// A config is evictable if it is: 1) not pinned, 2) its device exceeds minRetainedConfigs.
// Returns an empty string if no evictable config exists.
func getGlobalLRUCandidate(configsPerDevice map[string]int, sortedEntries []*ConfigMetadata, minRetainedConfigs int) (string, []*ConfigMetadata) {
	for i, entry := range sortedEntries {
		if entry.IsPinned {
			continue
		}
		if configsPerDevice[entry.DeviceID] > minRetainedConfigs {
			remaining := make([]*ConfigMetadata, 0, len(sortedEntries)-1)
			remaining = append(remaining, sortedEntries[:i]...)
			remaining = append(remaining, sortedEntries[i+1:]...)
			return entry.ConfigUUID, remaining
		}
	}
	return "", sortedEntries
}
