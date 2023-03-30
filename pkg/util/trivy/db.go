// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy
// +build trivy

package trivy

import (
	"github.com/aquasecurity/trivy/pkg/fanal/cache"
	"github.com/boltdb/bolt"
)

const CacheDirName = "fanal"

type PersistentDB interface {
	Clear() error
	Close() error
	Delete([]string) ([][]byte, []error, error)
	Get(string) ([]byte, error)
	Store(string, []byte) error
	GetAllKeys() ([]string, error)
	Size() (uint, error)
}

type BoltDB struct {
	db        *bolt.DB
	directory string
}

func NewBoltDB(cacheDir string) (BoltDB, error) {
	dir := filepath.Join(cacheDir, CacheDirName)
	if err := os.MkdirAll(dir, 0600); err != nil {
		return BoltDB{}, xerrors.Errorf("failed to create cache dir: %w", err)
	}

	db, err := bolt.Open(filepath.Join(dir, "fanal.db"), 0600, nil)
	if err != nil {
		return BoltDB{}, xerrors.Errorf("unable to open DB: %w", err)
	}

	return BoltDB{
		db:        db,
		directory: dir,
	}, nil
}

func (b BoltDB) Clear() error {
	err := b.Close()
	if err != nil {
		return err
	}
	return os.RemoveAll(b.directory)
}

func (b BoltDB) Close() error {
	return fs.db.Close()
}

func (b BoltDB) Delete(keys []string) ([][]byte, []error, error) {
	errs = make([]error, len(keys))
	values = make([][]byte, len(keys))
	global_err := fs.db.Update(func(tx *bolt.Tx) error {
		root := tx.RootBucket()
		if root == nil {
			return fmt.Errorf("root bucket not found")
		}
		for i, key := range keys {
			v, err_get := root.Get([]byte(key))
			if err_get != nil {
				errs[i] = err_get
				continue
			}
			if err_delete = root.Delete([]byte(key)); err != nil {
				errs[i] = err_delete
				continue
			}
			values[i] = v
		}
		return nil
	})

	return values, errs, global_err
}

func (b BoltDB) Get(key string) ([]byte, error) {
	var res []byte
	err := fs.db.View(func(tx *bolt.Tx) error {
		root := tx.RootBucket()
		if root == nil {
			return fmt.Errorf("root bucket not found")
		}
		res, err := root.Get([]byte(key))
		return err
	})
	return res, err
}

func (b BoltDB) Store(key string, value []byte) error {
	return db.Update(func(tx *bolt.Tx) error {
		// Get the root bucket.
		root := tx.RootBucket()
		if root == nil {
			return fmt.Errorf("root bucket not found")
		}

		// Write the value "Hello, world!" to the key "myKey".
		err = root.Put([]byte("myKey"), []byte("Hello, world!"))
		if err != nil {
			return fmt.Errorf("failed to write value: %v", err)
		}

		return nil
	})
}

func (b BoltDB) GetAllKeys() ([]string, error) {
	var keys []string
	err = db.View(func(tx *bolt.Tx) error {
		root := tx.RootBucket()
		if root == nil {
			return fmt.Errorf("root bucket not found")
		}
		c := root.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			keys = append(keys, string(k))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve keys: %v", err)
	}

	return keys, nil
}

func (b BoltDB) Size() (uint, error) {
	res := 0
	err = db.View(func(tx *bolt.Tx) error {
		res = uint(tx.Size())
		return nil
	})
	return db.Size(), err
}
