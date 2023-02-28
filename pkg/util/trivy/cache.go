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
)

var garbageCollectionFrequency = 10 * time.Minute
var garbageCollectionDiscardRatio = 0.5 // Recommended value: https://github.com/dgraph-io/badger/blob/3aa7bd6841baa884ff03f00f789317509dbcb051/db.go#L1192

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

	// Run garbage collection periodically. It's needed because Badger relies on
	// the client to run the garbage collection
	// (https://dgraph.io/docs/badger/get-started/#garbage-collection).
	gcTicker := time.NewTicker(garbageCollectionFrequency)
	go func() {
		for range gcTicker.C {
			if err := db.RunValueLogGC(garbageCollectionDiscardRatio); err != nil && err != badger.ErrNoRewrite {
				// ErrNoRewrite is returned when the GC runs fine but doesn't
				// result any cleanup. That's why we don't log anything in that
				// case.
				log.Warnf("error while running garbage collection in Badger: %s", err)
			}
		}
	}()

	return &BadgerCache{db: db, ttl: ttl, garbageCollectionTicker: gcTicker}, nil
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

func (cache *BadgerCache) PutArtifact(artifactID string, artifactInfo types.ArtifactInfo) error {
	artifactBytes, err := json.Marshal(artifactInfo)
	if err != nil {
		return fmt.Errorf("error converting artifact with ID %q to JSON: %w", artifactID, err)
	}

	err = cache.db.Update(func(txn *badger.Txn) error {
		entry := badger.NewEntry([]byte(artifactID), artifactBytes).WithTTL(cache.ttl)
		return txn.SetEntry(entry)
	})
	if err != nil {
		return fmt.Errorf("error saving artifact with ID %q into Badger cache: %w", artifactID, err)
	}

	return nil
}

func (cache *BadgerCache) PutBlob(blobID string, blobInfo types.BlobInfo) error {
	blobBytes, err := json.Marshal(blobInfo)
	if err != nil {
		return fmt.Errorf("error converting blob with ID %q to JSON: %w", blobID, err)
	}

	err = cache.db.Update(func(txn *badger.Txn) error {
		entry := badger.NewEntry([]byte(blobID), blobBytes).WithTTL(cache.ttl)
		return txn.SetEntry(entry)
	})
	if err != nil {
		return fmt.Errorf("error saving blob with ID %q into Badger cache: %w", blobID, err)
	}

	return nil
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

func (cache *BadgerCache) GetArtifact(artifactID string) (types.ArtifactInfo, error) {
	rawValue, err := cache.getRawValue(artifactID)

	if err != nil {
		return types.ArtifactInfo{}, fmt.Errorf("error getting artifact with ID %q from Badger cache: %w", artifactID, err)
	}

	var res types.ArtifactInfo
	if err := json.Unmarshal(rawValue, &res); err != nil {
		return types.ArtifactInfo{}, fmt.Errorf("JSON unmarshal error: %w", err)
	}

	return res, nil
}

func (cache *BadgerCache) GetBlob(blobID string) (types.BlobInfo, error) {
	rawValue, err := cache.getRawValue(blobID)

	if err != nil {
		return types.BlobInfo{}, fmt.Errorf("error getting blob with ID %q from Badger cache: %w", blobID, err)
	}

	var res types.BlobInfo
	if err := json.Unmarshal(rawValue, &res); err != nil {
		return types.BlobInfo{}, fmt.Errorf("JSON unmarshal error: %w", err)
	}

	return res, nil
}

func (cache *BadgerCache) Close() error {
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
