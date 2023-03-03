// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy
// +build trivy

package trivy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aquasecurity/trivy/pkg/fanal/cache"
	"github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/utils"
	"github.com/dgraph-io/badger/v3"
	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/telemetry"
)

var garbageCollectionTick = 10 * time.Minute
var garbageCollectionDiscardRatio = 0.5 // Recommended value: https://github.com/dgraph-io/badger/blob/3aa7bd6841baa884ff03f00f789317509dbcb051/db.go#L1192
var telemetryTick = 1 * time.Minute

type CacheProvider func() (cache.Cache, error)

func NewBoltCache(cacheDir string) (cache.Cache, error) {
	if cacheDir == "" {
		cacheDir = utils.DefaultCacheDir()
	}

	return cache.NewFSCache(cacheDir)
}

// BadgerCache implements the Trivy Cache interface. It's provided as an
// alternative to BoltDB, the cache used by default in Trivy. The main advantage
// of Badger is that it allows to specify a TTL for the objects stored in the
// cache.
type BadgerCache struct {
	db                      *badger.DB
	ttl                     time.Duration
	garbageCollectionTicker *time.Ticker
	telemetryTicker         *time.Ticker
}

// This is needed so that Badger logs using the same format as the Agent
type badgerLogger struct{}

func (l badgerLogger) Errorf(format string, params ...interface{}) {
	log.Errorf(format, params...)
}

func (l badgerLogger) Warningf(format string, params ...interface{}) {
	log.Warnf(format, params...)
}

func (l badgerLogger) Infof(format string, params ...interface{}) {
	log.Infof(format, params...)
}

func (l badgerLogger) Debugf(format string, params ...interface{}) {
	log.Debugf(format, params...)
}

func NewBadgerCache(cacheDir string, ttl time.Duration) (*BadgerCache, error) {
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "datadog-agent-sbom-cache")
	}

	db, err := badger.Open(badger.DefaultOptions(cacheDir).WithLogger(badgerLogger{}))
	if err != nil {
		return nil, fmt.Errorf("error creating Badger cache: %w", err)
	}

	telemetryTicker := time.NewTicker(telemetryTick)
	gcTicker := time.NewTicker(garbageCollectionTick)

	badgerCache := &BadgerCache{
		db:                      db,
		ttl:                     ttl,
		garbageCollectionTicker: gcTicker,
		telemetryTicker:         telemetryTicker,
	}

	go func() {
		for {
			select {
			case <-telemetryTicker.C:
				badgerCache.collectTelemetry()
			case <-gcTicker.C:
				// Run garbage collection periodically. It's needed because Badger relies on
				// the client to run the garbage collection
				// (https://dgraph.io/docs/badger/get-started/#garbage-collection).
				if err := db.RunValueLogGC(garbageCollectionDiscardRatio); err != nil && err != badger.ErrNoRewrite {
					// ErrNoRewrite is returned when the GC runs fine but doesn't
					// result any cleanup. That's why we don't log anything in that
					// case.
					log.Warnf("error while running garbage collection in Badger: %s", err)
				}
			}
		}
	}()

	return badgerCache, nil
}

func (cache *BadgerCache) MissingBlobs(artifactID string, blobIDs []string) (bool, []string, error) {
	var missingArtifact bool
	var missingBlobIDs []string

	err := cache.db.View(func(txn *badger.Txn) error {
		if _, err := txn.Get([]byte(artifactID)); err != nil {
			if err == badger.ErrKeyNotFound {
				missingArtifact = true
			} else {
				return err
			}
		}

		for _, blobID := range blobIDs {
			if _, err := txn.Get([]byte(blobID)); err != nil {
				if err == badger.ErrKeyNotFound {
					missingBlobIDs = append(missingBlobIDs, blobID)
				} else {
					return err
				}
			}
		}

		return nil
	})
	if err != nil {
		return false, nil, fmt.Errorf("error getting missing blobs from Badger cache: %w", err)
	}

	return missingArtifact, missingBlobIDs, nil
}

type cachedObject interface {
	types.ArtifactInfo | types.BlobInfo
}

// Cannot be a BadgerCache method because methods cannot have type parameters
func badgerCachePut[T cachedObject](cache *BadgerCache, id string, info T) error {
	objectBytes, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("error converting object with ID %q to JSON: %w", id, err)
	}

	err = cache.db.Update(func(txn *badger.Txn) error {
		entry := badger.NewEntry([]byte(id), objectBytes).WithTTL(cache.ttl)
		return txn.SetEntry(entry)
	})
	if err != nil {
		return fmt.Errorf("error saving object with ID %q into Badger cache: %w", id, err)
	}

	return nil
}

func (cache *BadgerCache) PutArtifact(artifactID string, artifactInfo types.ArtifactInfo) error {
	return badgerCachePut(cache, artifactID, artifactInfo)
}

func (cache *BadgerCache) PutBlob(blobID string, blobInfo types.BlobInfo) error {
	return badgerCachePut(cache, blobID, blobInfo)
}

func (cache *BadgerCache) DeleteBlobs(blobIDs []string) error {
	var errs error

	err := cache.db.Update(func(txn *badger.Txn) error {
		for _, blobID := range blobIDs {
			if err := txn.Delete([]byte(blobID)); err != nil {
				errs = multierror.Append(errs, err)
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("error deleting blobs from Badger cache: %w", err)
	}

	return errs
}

// Cannot be a BadgerCache method because methods cannot have type parameters
func badgerCacheGet[T cachedObject](cache *BadgerCache, id string) (T, error) {
	rawValue, err := cache.getRawValue(id)
	var empty T

	if err != nil {
		return empty, fmt.Errorf("error getting object with ID %q from Badger cache: %w", id, err)
	}

	var res T
	if err := json.Unmarshal(rawValue, &res); err != nil {
		return empty, fmt.Errorf("JSON unmarshal error: %w", err)
	}

	return res, nil
}

func (cache *BadgerCache) GetArtifact(artifactID string) (types.ArtifactInfo, error) {
	return badgerCacheGet[types.ArtifactInfo](cache, artifactID)
}

func (cache *BadgerCache) GetBlob(blobID string) (types.BlobInfo, error) {
	return badgerCacheGet[types.BlobInfo](cache, blobID)
}

func (cache *BadgerCache) Close() error {
	cache.telemetryTicker.Stop()
	cache.garbageCollectionTicker.Stop()

	if err := cache.db.Close(); err != nil {
		return fmt.Errorf("error closing the Badger cache: %w", err)
	}

	return nil
}

func (cache *BadgerCache) Clear() error {
	if err := cache.db.DropAll(); err != nil {
		return fmt.Errorf("error clearing the Badger cache: %w", err)
	}

	return nil
}

func (cache *BadgerCache) getRawValue(key string) ([]byte, error) {
	var rawValue []byte
	err := cache.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			rawValue = append([]byte{}, val...)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return rawValue, nil
}

func (cache *BadgerCache) collectTelemetry() {
	lsmSize, vlogSize := cache.db.Size()
	telemetry.SBOMCacheMemSize.Set(float64(lsmSize))
	telemetry.SBOMCacheDiskSize.Set(float64(vlogSize))
}
