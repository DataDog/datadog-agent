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
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
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
	db         *bbolt.DB
	lock       sync.RWMutex
	compressor compression.Compressor
}

// Open creates a new ConfigStore and initializes the underlying boltDB + required buckets
func Open(path string) (*ConfigStore, error) {
	db, err := bbolt.Open(path, ownerRWFileMode, &bbolt.Options{
		Timeout: databaseLockTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open NCM bbolt config store at %s: %w", path, err)
	}

	cs := &ConfigStore{
		db:         db,
		compressor: selector.NewCompressor(compression.ZstdKind, 3), // Level 3 is default for compression, can tune iteratively
	}

	// Create the buckets when we first open
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

// Size returns the size of the database file in bytes.
func (cs *ConfigStore) Size() (int64, error) {
	var size int64
	err := cs.view(func(tx *bbolt.Tx) error {
		size = tx.Size()
		return nil
	})
	return size, err
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
	// Setup + marshal everything first (does not require DB lock)
	configUUID := uuid.New().String()
	now := time.Now().Unix()
	rawHash := hashConfig(rawConfig)

	// Raw text
	rawConfigJSON, err := json.Marshal(rawConfig)
	if err != nil {
		return "", fmt.Errorf("marshal raw config error: %w", err)
	}
	compressedRawConfigJSON, err := cs.compressor.Compress((rawConfigJSON))
	if err != nil {
		return "", fmt.Errorf("compress raw config error: %w", err)
	}

	// Blocks / raw text
	blocksJSON, err := json.Marshal(blocks)
	if err != nil {
		return "", fmt.Errorf("marshal config blocks error: %w", err)
	}
	compressedBlocksJSON, err := cs.compressor.Compress(blocksJSON)
	if err != nil {
		return "", fmt.Errorf("compress config blocks error: %w", err)
	}

	// Metadata
	metadata := ConfigMetadata{
		ConfigUUID:     configUUID,
		DeviceID:       deviceID,
		ConfigType:     configType,
		CapturedAt:     now,
		LastAccessedAt: now,
		RawHash:        rawHash,
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

	var existingConfigID string

	// Update the DB with all the JSONs
	err = cs.update(func(tx *bbolt.Tx) error {
		// Check that this is a new config for the DB - does the hash match the last stored config for this device?
		// TODO: optimization for consideration: utilizing a composite key that is made up of
		// config_type | device_id | timestamp | uuid (or using this with another bucket to emulate an index)
		existingConfigID, err = cs.checkDuplicateInTx(tx, deviceID, configType, rawHash)
		if err != nil {
			return fmt.Errorf("duplicate check error: %w", err)
		}
		if existingConfigID != "" {
			return nil // This config matched the latest, let's not make a new entry
		}

		key := []byte(configUUID) // TODO: include more for prefix searches?
		if err := tx.Bucket([]byte(rawConfigBucket)).Put(key, compressedRawConfigJSON); err != nil {
			return err
		}
		if err := tx.Bucket([]byte(configBlocksBucket)).Put(key, compressedBlocksJSON); err != nil {
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
	if existingConfigID != "" {
		return existingConfigID, nil
	}
	return configUUID, nil
}

// checkDuplicateInTx contains the inner logic for iterating through the metadata bucket (currently keyed by UUID)
// and checks for configs that match the device ID and config type (e.g. default:10.0.0.1, "running")
// and compares the hashes with the incoming config retrieved to help check if we need to store it
// TODO: nice to have optimization since we check duplicates more than we'd check by exact UUID is having a composite key / prefix scan
func (cs *ConfigStore) checkDuplicateInTx(tx *bbolt.Tx, deviceID string, configType string, rawHash string) (string, error) {
	var latest *ConfigMetadata
	err := tx.Bucket([]byte(metadataBucket)).ForEach(func(_, v []byte) error {
		var current ConfigMetadata
		if err := json.Unmarshal(v, &current); err != nil {
			return err
		}
		if current.DeviceID == deviceID && current.ConfigType == configType {
			if latest == nil || current.CapturedAt > latest.CapturedAt || (current.CapturedAt == latest.CapturedAt && current.ConfigUUID > latest.ConfigUUID) {
				latest = &current
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	// compare the hashes if a latest was found
	if latest != nil && latest.RawHash == rawHash {
		return latest.ConfigUUID, nil
	}
	return "", nil
}

// CheckDuplicate is the wrapper around the checkDuplicateInTx function that contains the logic including locking the DB
func (cs *ConfigStore) CheckDuplicate(deviceID string, configType string, rawHash string) (string, error) {
	var configID string
	err := cs.view(func(tx *bbolt.Tx) error {
		var txErr error
		configID, txErr = cs.checkDuplicateInTx(tx, deviceID, configType, rawHash)
		return txErr
	})
	return configID, err
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
		decompressedRawConfig, err := cs.compressor.Decompress(rawConfigBytes)
		if err != nil {
			return fmt.Errorf("decompress raw config error: %w", err)
		}
		if err := json.Unmarshal(decompressedRawConfig, &rawConfig); err != nil {
			return fmt.Errorf("unmarshal raw config error: %w", err)
		}

		// Unmarshal blocks
		blocksBytes := tx.Bucket([]byte(configBlocksBucket)).Get(key)
		if blocksBytes == nil {
			return fmt.Errorf("blocks not found for UUID: %s", configUUID)
		}
		decompressedBlocks, err := cs.compressor.Decompress(blocksBytes)
		if err != nil {
			return fmt.Errorf("decompress config blocks error: %w", err)
		}
		if err := json.Unmarshal(decompressedBlocks, &blocks); err != nil {
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

// DeleteConfig deletes all data associated with the given key (config UUID) from each bucket
func (cs *ConfigStore) DeleteConfig(key string) error {
	return cs.update(func(tx *bbolt.Tx) error {
		bKey := []byte(key)

		// Check existence via metadata bucket before deleting
		if tx.Bucket([]byte(metadataBucket)).Get(bKey) == nil {
			return fmt.Errorf("config not found for key: %s", key)
		}

		for _, bucketName := range []string{rawConfigBucket, configBlocksBucket, metadataBucket, secretsBucket} {
			if err := tx.Bucket([]byte(bucketName)).Delete(bKey); err != nil {
				return fmt.Errorf("error deleting config from bbolt bucket %s: %w", bucketName, err)
			}
		}
		return nil
	})
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

// getEvictableExceedingMax returns UUIDs of configs to evict due to the per-device cap N.
// For each device whose total config count exceeds N, evict the oldest evictable configs.
// This function should be called first before calling for the global LRU candidate.
// It does not mutate the index; call updateEvictionIndex with the returned UUIDs to apply changes.
func getEvictableExceedingMax(configsPerDevice map[string]int, sortedEntries []*ConfigMetadata, maxRetainedConfigs int) []string {
	var evictable []string
	pendingEvictions := make(map[string]int)

	for _, entry := range sortedEntries {
		if entry.IsPinned {
			continue
		}
		if configsPerDevice[entry.DeviceID]-pendingEvictions[entry.DeviceID] <= maxRetainedConfigs {
			continue
		}
		evictable = append(evictable, entry.ConfigUUID)
		pendingEvictions[entry.DeviceID]++
	}
	return evictable
}

// getGlobalLRUCandidate returns the UUID of the single oldest evictable config (rule 3).
// A config is evictable if it is: 1) not pinned, 2) its device exceeds minRetainedConfigs.
// Returns an empty string if no evictable config exists.
// It does not mutate the index; call updateEvictionIndex with the returned UUID to apply changes.
func getGlobalLRUCandidate(configsPerDevice map[string]int, sortedEntries []*ConfigMetadata, minRetainedConfigs int) string {
	for _, entry := range sortedEntries {
		if entry.IsPinned {
			continue
		}
		if configsPerDevice[entry.DeviceID] > minRetainedConfigs {
			return entry.ConfigUUID
		}
	}
	return ""
}

// updateEvictionIndex removes a single config key from both index data structures.
// Both the updated configsPerDevice map and sortedEntries slice are returned to make
// it explicit that both structures are outputs of this operation.
func updateEvictionIndex(configsPerDevice map[string]int, sortedEntries []*ConfigMetadata, key string) (map[string]int, []*ConfigMetadata) {

	var remaining []*ConfigMetadata

	for i, entry := range sortedEntries {
		if entry.ConfigUUID == key {
			configsPerDevice[entry.DeviceID]--
			remaining = append(remaining, sortedEntries[i+1:]...)
			return configsPerDevice, remaining
		}
		remaining = append(remaining, entry)
	}

	return configsPerDevice, remaining
}

// getEvictionCandidates builds the eviction index and returns a single ordered list of
// config UUIDs to evict. Per-device cap violations come first, followed by global LRU
// candidates (oldest first). LRU candidates are only collected when the DB size exceeds maxSize.
func (cs *ConfigStore) getEvictionCandidates(minRetainedConfigs int, maxRetainedConfigs int, maxSize int64) ([]string, error) {
	configsPerDevice, sortedEntries, err := cs.buildEvictionIndex()
	if err != nil {
		return nil, err
	}

	candidates := getEvictableExceedingMax(configsPerDevice, sortedEntries, maxRetainedConfigs)
	for _, uuid := range candidates {
		configsPerDevice, sortedEntries = updateEvictionIndex(configsPerDevice, sortedEntries, uuid)
	}

	size, err := cs.Size()
	if err != nil {
		return nil, err
	}
	if size > maxSize {
		for {
			candidate := getGlobalLRUCandidate(configsPerDevice, sortedEntries, minRetainedConfigs)
			if candidate == "" {
				break
			}
			candidates = append(candidates, candidate)
			configsPerDevice, sortedEntries = updateEvictionIndex(configsPerDevice, sortedEntries, candidate)
		}
	}

	return candidates, nil
}

// evictConfigs deletes the configs identified by the given UUIDs.
func (cs *ConfigStore) evictConfigs(uuids []string) error {
	for _, uuid := range uuids {
		if err := cs.DeleteConfig(uuid); err != nil {
			return err
		}
	}
	return nil
}
