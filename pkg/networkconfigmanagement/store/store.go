// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ncm

// Package store is provides persistent local storage for network device configurations (for NCM)
// utilizing bbolt - enabling rollback of configs w/o sending data to the Datadog backend
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.etcd.io/bbolt"

	ncmreport "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/report"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// Bucket names for bbolt
	rawConfigBucket = "raw_config" // TODO: temporary bucket, a hybrid approach until the blocks logic is ironed out
	metadataBucket  = "metadata"

	// DB configurations
	ownerRWFileMode     = 0600 // only the owner can read/write
	databaseLockTimeout = 1 * time.Second
)

type configStore struct {
	db         *bbolt.DB
	lock       sync.RWMutex
	compressor compression.Compressor
}

var _ ConfigStore = (*configStore)(nil)

// Open creates a new ConfigStore and initializes the underlying boltDB + required buckets
func Open(path string) (ConfigStore, error) {
	db, err := bbolt.Open(path, ownerRWFileMode, &bbolt.Options{
		Timeout: databaseLockTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open NCM bbolt config store at %s: %w", path, err)
	}

	cs := &configStore{
		db:         db,
		compressor: selector.NewCompressor(compression.ZstdKind, 3), // Level 3 is default for compression, can tune iteratively
	}

	// Create the buckets when we first open
	err = cs.update(func(tx *bbolt.Tx) error {
		for _, name := range []string{rawConfigBucket, metadataBucket} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return fmt.Errorf("error creating bucket %s: %w", name, err)
			}
		}
		return nil
	})
	if err != nil {
		cs.Close(context.TODO())
		return nil, err
	}

	return cs, nil
}

// Close cleans up / releases DB resources
func (cs *configStore) Close(_ context.Context) error {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	return cs.db.Close()
}

// Base helper transaction functions for the DB

// view wraps the bbolt View transaction with a read lock (for ease of use)
func (cs *configStore) view(fn func(tx *bbolt.Tx) error) error {
	cs.lock.RLock()
	defer cs.lock.RUnlock()
	return cs.db.View(fn)
}

// update wraps the bbolt Update transaction with a write lock (for ease of use)
func (cs *configStore) update(fn func(tx *bbolt.Tx) error) error {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	return cs.db.Update(fn)
}

// NCM-specific transaction functions

// StoreConfig is responsible for checking if the config for the device is new,
// if so, it will create a new entry in each bucket (for the config, metadata, and secrets)
func (cs *configStore) StoreConfig(deviceID string, configType ncmreport.ConfigType, rawConfig string) (string, error) {
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
		if err := tx.Bucket([]byte(metadataBucket)).Put(key, metadataJSON); err != nil {
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
func (cs *configStore) checkDuplicateInTx(tx *bbolt.Tx, deviceID string, configType ncmreport.ConfigType, rawHash string) (string, error) {
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
func (cs *configStore) CheckDuplicate(deviceID string, configType ncmreport.ConfigType, rawHash string) (string, error) {
	var configID string
	err := cs.view(func(tx *bbolt.Tx) error {
		var txErr error
		configID, txErr = cs.checkDuplicateInTx(tx, deviceID, configType, rawHash)
		return txErr
	})
	return configID, err
}

// GetConfig retrieves all the data associated with a config given its UUID
func (cs *configStore) GetConfig(configUUID string) (string, *ConfigMetadata, error) {
	var rawConfig string
	var metadata ConfigMetadata

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

		// Unmarshal metadata
		metadataBytes := tx.Bucket([]byte(metadataBucket)).Get(key)
		if metadataBytes == nil {
			return fmt.Errorf("metadata not found for UUID: %s", configUUID)
		}
		if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
			return fmt.Errorf("unmarshal metadata error: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", nil, err
	}

	return rawConfig, &metadata, nil
}

// DeleteConfig deletes all data associated with the given key (config UUID) from each bucket
func (cs *configStore) DeleteConfig(key string) error {
	return cs.update(func(tx *bbolt.Tx) error {
		bKey := []byte(key)

		// Check existence via metadata bucket before deleting
		if tx.Bucket([]byte(metadataBucket)).Get(bKey) == nil {
			return fmt.Errorf("config not found for key: %s", key)
		}

		for _, bucketName := range []string{rawConfigBucket, metadataBucket} {
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
