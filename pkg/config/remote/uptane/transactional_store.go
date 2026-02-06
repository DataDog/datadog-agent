// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"fmt"
	"slices"
	"sync"

	"github.com/pkg/errors"
	"go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// dbBucket contains the data of the bucket
// if the content it's nil it means the file must be deleted, otherwise it contains data
type dbBucket map[string][]byte

// transactionalStore persists all the target files present in the current director targets.json
// All writes go to the in-memory structure until the `commit` method is called.
// All reads first read the in-memory data and fallback to the on disk one.
// A call to `commit` will flush all changes to the DB in a single transaction.
type transactionalStore struct {
	// underlying database where we apply many changes atomically
	db *bbolt.DB

	// lock to access the cached data and bbolt db
	lock sync.RWMutex

	// bucket -> key -> data
	cachedData map[string]dbBucket
}

// Represents a transaction around the store.
// It doesn't provide any locking, as it's all managed by View/Update calls
type transaction struct {
	store *transactionalStore
}

// Metadata needed in order to recreate boltDB
type Metadata struct {
	Path         string
	AgentVersion string
	APIKey       string
	URL          string
}

// NewTransactionalStore creates a new transactional store comprised of creating the underlying boltDB
func newTransactionalStore(metadata *Metadata) (*transactionalStore, error) {
	// transactional store should be in charge of opening/closing boltDB
	db, err := openCacheDB(metadata.Path, metadata.AgentVersion, metadata.APIKey, metadata.URL)
	if err != nil {
		return nil, err
	}
	s := &transactionalStore{
		db:         db,
		cachedData: make(map[string]dbBucket),
	}
	return s, nil
}

// RecreateTransactionalStore uses the metadata & path from the existing TS boltDB to open a new one & clear cachedData
func recreateTransactionalStore(metadata *Metadata) (*transactionalStore, error) {
	db, err := recreate(metadata.Path, metadata.AgentVersion, metadata.APIKey, metadata.URL)
	if err != nil {
		return nil, err
	}
	s := &transactionalStore{
		db:         db,
		cachedData: make(map[string]dbBucket),
	}
	return s, nil
}

// getTSMetadata gets metadata from existing transactional db and then closes the existing underlying boltDB
func (ts *transactionalStore) getTSMetadata() (*Metadata, error) {
	metadata, err := getMetadata(ts.db)
	if err != nil {
		return nil, fmt.Errorf("could not read metadata from the database: %w", err)
	}
	path := ts.db.Path()

	err = ts.db.Close()
	if err != nil {
		return nil, err
	}

	return &Metadata{
		Path:         path,
		AgentVersion: metadata.Version,
		APIKey:       metadata.APIKeyHash,
		URL:          metadata.URL,
	}, nil
}

// getMemBucket returns a refence to the in-memory bucket
func (ts *transactionalStore) getMemBucket(bucketName string) dbBucket {
	cachedBucket, ok := ts.cachedData[bucketName]
	if !ok {
		cachedBucket = make(dbBucket)
		ts.cachedData[bucketName] = cachedBucket
	}
	return cachedBucket
}

type pathData struct {
	path string
	data []byte
}

func (ts *transactionalStore) getAll(bucketName string) ([]pathData, error) {
	seenBlobs := map[string]struct{}{}
	blobs := []pathData{}

	for path, data := range ts.getMemBucket(bucketName) {
		seenBlobs[path] = struct{}{}
		if len(data) == 0 {
			// if the path is seen but the data is empty in the in-memory state, it means we've "deleted" it
			// so we should not include in the result, and also not try to read it from the db.
			continue
		}
		blobs = append(blobs, pathData{path, data})
	}

	err := ts.db.View(func(tx *bbolt.Tx) error {
		metaBucket := tx.Bucket([]byte(bucketName))
		if metaBucket == nil {
			return nil
		}

		cursor := metaBucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			_, ok := seenBlobs[string(k)]
			if ok {
				continue
			}

			if len(v) == 0 {
				continue
			}

			tmp := make([]byte, len(v))
			copy(tmp, v)
			blobs = append(blobs, pathData{string(k), tmp})
		}
		return nil
	})

	return blobs, err
}

// transaction types
func (ts *transactionalStore) view(fn func(*transaction) error) error {
	ts.lock.RLock()
	defer ts.lock.RUnlock()
	return fn(&transaction{ts})
}

func (ts *transactionalStore) update(fn func(*transaction) error) error {
	ts.lock.Lock()
	defer ts.lock.Unlock()
	err := fn(&transaction{ts})
	return err
}

func (t *transaction) getAll(bucketName string) ([]pathData, error) {
	return t.store.getAll(bucketName)
}

func (t *transaction) pruneTargetFiles(bucketName string, keptPaths []string) error {
	kept := make(map[string]struct{})
	for _, k := range keptPaths {
		kept[k] = struct{}{}
	}

	// delete from in-memory
	memBucket := t.store.getMemBucket(bucketName)
	for path := range memBucket {
		if _, keep := kept[path]; !keep {
			t.delete(bucketName, path)
		}
	}

	// delete in-memory based on files in the DB
	return t.store.db.View(func(tx *bbolt.Tx) error {
		targetBucket := tx.Bucket([]byte(bucketName))
		if targetBucket == nil {
			return nil
		}
		cursor := targetBucket.Cursor()
		for k, _ := cursor.First(); k != nil; k, _ = cursor.Next() {
			path := string(k)
			if _, keep := kept[path]; !keep {
				t.delete(bucketName, path)
			}
		}
		return nil
	})
}

// commit all data from each bucket to the underlying database
func (ts *transactionalStore) commit() error {
	ts.lock.Lock()
	defer ts.lock.Unlock()
	err := ts.db.Update(func(tx *bbolt.Tx) error {
		for bucketName, memBucket := range ts.cachedData {
			bucket, err := tx.CreateBucketIfNotExists([]byte(bucketName))
			if err != nil {
				return err
			}
			for path, data := range memBucket {
				if len(data) == 0 {
					err := bucket.Delete([]byte(path))
					if err != nil {
						return err
					}
				} else {
					err := bucket.Put([]byte(path), data)
					if err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		log.Debugf("Failed Commit: %v", err)
	} else {
		log.Debugf("Commit successful. %d keys", len(ts.cachedData))
	}
	ts.cachedData = make(map[string]dbBucket)

	return err
}

// removes all cached changes
func (ts *transactionalStore) rollback() {
	ts.lock.Lock()
	defer ts.lock.Unlock()
	if len(ts.cachedData) > 0 {
		log.Debugf("Rollback of %d keys", len(ts.cachedData))
		ts.cachedData = make(map[string]dbBucket)
	}
}

func (ts *transactionalStore) Close() error {
	ts.lock.Lock()
	defer ts.lock.Unlock()
	return ts.db.Close()
}

// for test in service pkg
func (ts *transactionalStore) GetPath() string {
	ts.lock.Lock()
	defer ts.lock.Unlock()
	return ts.db.Path()
}

func (t *transaction) put(bucketName string, path string, data []byte) {
	log.Debugf("Putting %s in bucket %s", path, bucketName)
	bucket := t.store.getMemBucket(bucketName)
	bucket[path] = data
}

func (t *transaction) delete(bucketName string, path string) {
	log.Debugf("Deleting %s from bucket %s", path, bucketName)
	bucket := t.store.getMemBucket(bucketName)
	bucket[path] = nil
}

func (t *transaction) get(bucketName string, path string) ([]byte, error) {
	log.Debugf("Get %s from bucket %s", path, bucketName)
	bucket := t.store.getMemBucket(bucketName)

	// check if it's present in the in-memory cache
	data, ok := bucket[path]
	if ok {
		return data, nil
	}

	// fallback to DB access
	err := t.store.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return nil
		}
		data = slices.Clone(bucket.Get([]byte(path)))
		return nil
	})

	if len(data) == 0 {
		err = errors.Wrapf(err, "File empty or not found: %s in bucket %s", path, bucketName)
	}

	return data, err
}
