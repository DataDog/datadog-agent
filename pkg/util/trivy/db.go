// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package trivy

import (
	"fmt"
	"os"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

const (
	// cacheDirName is the name of the directory where the cache files are stored.
	cacheDirName = "fanal"
	// boltBucket is the name of the BoltDB bucket that stores key-value pairs.
	boltBucket = "boltdb"
)

// onDeleteCallback describes a callback function that is called before deleting an entry from a PersistentDB.
type onDeleteCallback = func(key string, value []byte) error

// BoltDB implements the PersistentDB interface. It holds a bolt.DB instance and the storage directory.
type BoltDB struct {
	db        *bolt.DB
	directory string
}

// NewBoltDB creates a new BoltDB instance.
func NewBoltDB(cacheDir string) (BoltDB, error) {
	dir := filepath.Join(cacheDir, cacheDirName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return BoltDB{}, fmt.Errorf("failed to create cache dir: %v", err)
	}

	db, err := bolt.Open(filepath.Join(dir, "fanal.db"), 0600, nil)
	if err != nil {
		return BoltDB{}, fmt.Errorf("unable to open DB: %v", err)
	}

	if err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(boltBucket)); err != nil {
			return fmt.Errorf("unable to create %s bucket: %v", boltBucket, err)
		}
		return nil
	}); err != nil {
		return BoltDB{}, err
	}

	return BoltDB{
		db:        db,
		directory: dir,
	}, nil
}

// Clear clears the cache directory.
func (b BoltDB) Clear() error {
	if err := b.Close(); err != nil {
		return err
	}
	return os.RemoveAll(b.directory)
}

// Close closes the db.
func (b BoltDB) Close() error {
	return b.db.Close()
}

// Delete deletes the given keys from the database and calls the callback for each deleted key-value pair.
// It returns an error if the transaction fails and returns nil if the key doesn't exist.
func (b BoltDB) Delete(keys []string, callback onDeleteCallback) error {
	err := b.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(boltBucket))
		if err != nil {
			return err
		}
		for _, key := range keys {
			value := bucket.Get([]byte(key))
			if err = bucket.Delete([]byte(key)); err != nil {
				return err
			}
			if err = callback(key, value); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

// Get retrieves the value attached to the given key.
func (b BoltDB) Get(key string) ([]byte, error) {
	var res []byte
	return res, b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(boltBucket))
		if bucket == nil {
			return fmt.Errorf("bucket %s not found", boltBucket)
		}
		res = bucket.Get([]byte(key))
		return nil
	})
}

// Store inserts a key in the database.
func (b BoltDB) Store(key string, value []byte) error {
	return b.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(boltBucket))
		if err != nil {
			return err
		}
		return bucket.Put([]byte(key), value)
	})
}

// ForEach runs a function for each key in the db.
func (b BoltDB) ForEach(f func(string, []byte) error) error {
	return b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(boltBucket))
		if bucket == nil {
			return fmt.Errorf("bucket %s not found", boltBucket)
		}
		return bucket.ForEach(func(k []byte, v []byte) error { return f(string(k), v) })
	})
}

// Size returns the size of the database file.
func (b BoltDB) Size() (uint, error) {
	var res uint
	return res, b.db.View(func(tx *bolt.Tx) error {
		res = uint(tx.Size())
		return nil
	})
}
