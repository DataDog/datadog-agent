// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trivy

import (
	"fmt"
	"os"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
)

const CacheDirName = "fanal"
const boltBucket = "boltdb"

type PersistentDB interface {
	Clear() error
	Close() error
	Delete([]string) ([][]byte, error)
	Get(string) ([]byte, error)
	Store(string, []byte) error
	ForEach(func(string, []byte) error) error
	Size() (uint, error)
}

type BoltDB struct {
	db        *bolt.DB
	directory string
}

func NewBoltDB(cacheDir string) (BoltDB, error) {
	dir := filepath.Join(cacheDir, CacheDirName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return BoltDB{}, fmt.Errorf("failed to create cache dir: %v", err)
	}

	db, err := bolt.Open(filepath.Join(dir, "fanal.db"), 0600, &bolt.Options{
		NoGrowSync:     true,
		NoFreelistSync: true,
	})
	if err != nil {
		return BoltDB{}, fmt.Errorf("unable to open DB: %v", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(boltBucket)); err != nil {
			return fmt.Errorf("unable to create %s bucket: %v", boltBucket, err)
		}
		return nil
	})

	return BoltDB{
		db:        db,
		directory: dir,
	}, nil
}

func (b BoltDB) Clear() error {
	if err := b.Close(); err != nil {
		return err
	}
	return os.RemoveAll(b.directory)
}

func (b BoltDB) Close() error {
	return b.db.Close()
}

func (b BoltDB) Delete(keys []string) ([][]byte, error) {
	var errs []error
	values := make([][]byte, len(keys))
	errs = append(errs, b.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(boltBucket))
		if err != nil {
			return err
		}
		for i, key := range keys {
			v := bucket.Get([]byte(key))
			values[i] = v
			if cerr := bucket.Delete([]byte(key)); cerr != nil {
				errs = append(errs, cerr)
			}
		}
		return nil
	}))
	return values, kerrors.NewAggregate(errs)
}

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

func (b BoltDB) Store(key string, value []byte) error {
	return b.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(boltBucket))
		if err != nil {
			return err
		}
		return bucket.Put([]byte(key), value)
	})
}

func (b BoltDB) ForEach(f func(string, []byte) error) error {
	return b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(boltBucket))
		if bucket == nil {
			return fmt.Errorf("bucket %s not found", boltBucket)
		}
		return bucket.ForEach(func(k []byte, v []byte) error { return f(string(k), v) })
	})
}

func (b BoltDB) Size() (uint, error) {
	var res uint
	return res, b.db.View(func(tx *bolt.Tx) error {
		res = uint(tx.Size())
		return nil
	})
}
