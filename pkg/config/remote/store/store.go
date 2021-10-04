// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package store

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	fmt "fmt"
	"math"
	"os"

	"github.com/hashicorp/go-multierror"
	"go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/security/log"
)

// Store allows storing configuration in a boltdb database
type Store struct {
	path          string
	db            *bbolt.DB
	maxBucketSize int
	configBucket  string
}

// Close the store
func (s *Store) Close() error {
	return s.db.Close()
}

// keyFromVersion returns the boltdb key from a version.
// boltdb entries are byte sorted so we need to convert it to big endian.
func keyFromVersion(version uint64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, version)
	return key
}

// StoreConfig inserts a configuration into the database
func (s *Store) StoreConfig(product string, config *pbgo.ConfigResponse) error {
	if config == nil {
		return errors.New("cannot store empty configuration")
	}

	err := s.db.Update(func(tx *bbolt.Tx) error {
		configBucket, err := tx.CreateBucketIfNotExists([]byte(s.configBucket))
		if err != nil {
			return err
		}

		productBucket, err := configBucket.CreateBucketIfNotExists([]byte(product))
		if err != nil {
			return err
		}

		key := keyFromVersion(config.DirectoryTargets.Version)
		data, err := json.Marshal(config)
		if err != nil {
			return err
		}

		return productBucket.Put(key, data)
	})

	if err != nil {
		return err
	}

	log.Debugf("Prune %s bucket", product)
	return s.db.Update(func(tx *bbolt.Tx) error {
		configBucket, err := s.getConfigBucket(tx)
		if err != nil {
			return err
		}

		productBucket := configBucket.Bucket([]byte(product))
		if productBucket == nil {
			return fmt.Errorf("could not find bucket %s: %w", product, bbolt.ErrBucketNotFound)
		}

		s.pruneBucket(productBucket)
		return nil
	})
}

func (s *Store) getConfigBucket(tx *bbolt.Tx) (*bbolt.Bucket, error) {
	configBucket := tx.Bucket([]byte(s.configBucket))
	if configBucket == nil {
		return nil, fmt.Errorf("could not find bucket %s: %w", s.configBucket, bbolt.ErrBucketNotFound)
	}
	return configBucket, nil
}

// pruneBucket purges the bucket entries to keep only
// maxBucketSize entries at maximum
func (s *Store) pruneBucket(bucket *bbolt.Bucket) {
	stats := bucket.Stats()
	size := stats.KeyN
	cursor := bucket.Cursor()
	var errs *multierror.Error
	for k, _ := cursor.First(); k != nil && size > s.maxBucketSize; k, _ = cursor.Next() {
		if err := cursor.Delete(); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("failed to prune key %d: %w", k, err))
		}
		size--
	}

	if errs.ErrorOrNil() != nil {
		log.Errorf("failed to prune bucket: %w", errs)
	}
}

// GetLastConfig returns the last configuration for a product
func (s *Store) GetLastConfig(product string) (config *pbgo.ConfigResponse, err error) {
	configs, err := s.getConfigs(product, 1)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, fmt.Errorf("no last configuration found for %s", product)
	}
	return configs[0], nil
}

// GetConfigs returns all the stored configurations for a product
func (s *Store) GetConfigs(product string) (configs []*pbgo.ConfigResponse, err error) {
	return s.getConfigs(product, math.MaxInt32)
}

// GetProducts returns all the product
func (s *Store) GetProducts() ([]string, error) {
	var products []string
	err := s.db.View(func(tx *bbolt.Tx) error {
		configBucket, err := s.getConfigBucket(tx)
		if err != nil {
			if !errors.Is(err, bbolt.ErrBucketNotFound) {
				return err
			}
			return nil
		}

		return configBucket.ForEach(func(name []byte, _ []byte) error {
			products = append(products, string(name))
			return nil
		})
	})
	return products, err
}

func (s *Store) getConfigs(product string, maxConfig int) (configs []*pbgo.ConfigResponse, err error) {
	err = s.db.View(func(tx *bbolt.Tx) error {
		configBucket, err := s.getConfigBucket(tx)
		if err != nil {
			if !errors.Is(err, bbolt.ErrBucketNotFound) {
				return err
			}
			return nil
		}

		productBucket := configBucket.Bucket([]byte(product))
		if productBucket == nil {
			return fmt.Errorf("could not find bucket %s: %w", product, bbolt.ErrBucketNotFound)
		}

		cursor := productBucket.Cursor()
		for k, v := cursor.First(); k != nil && maxConfig > 0; k, v = cursor.Next() {
			var config pbgo.ConfigResponse
			if err := json.Unmarshal(v, &config); err != nil {
				return err
			}
			configs = append(configs, &config)
			maxConfig--
		}
		return nil
	})

	return
}

// GetMeta returns a map of all the metadata of a repository
func (s *Store) GetMeta(repository string) (map[string]json.RawMessage, error) {
	meta := make(map[string]json.RawMessage)
	err := s.db.View(func(tx *bbolt.Tx) error {
		configBucket, err := s.getConfigBucket(tx)
		if err != nil {
			return err
		}

		metaBucket := configBucket.Bucket([]byte(repository))
		if metaBucket == nil {
			return fmt.Errorf("could not find bucket '%s': %w", repository, bbolt.ErrBucketNotFound)
		}

		cursor := metaBucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			tmp := make([]byte, len(v))
			copy(tmp, v)
			meta[string(k)] = json.RawMessage(tmp)
		}

		return nil
	})
	return meta, err
}

// SetMeta stores a metadata
// The meta store is unbounded but it isn't a problem as theclient always
// store metas with ROLE.json. We won't have an infinite number of delegations
// meta as we only add manually 1 by product and they don't need to be cleaned up.
func (s *Store) SetMeta(repository string, name string, meta json.RawMessage) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		configBucket, err := tx.CreateBucketIfNotExists([]byte(s.configBucket))
		if err != nil {
			return err
		}

		metaBucket, err := configBucket.CreateBucketIfNotExists([]byte(repository))
		if err != nil {
			return err
		}

		return metaBucket.Put([]byte(name), meta)
	})
}

// DeleteMeta deletes a metadata
func (s *Store) DeleteMeta(repository string, name string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		configBucket, err := tx.CreateBucketIfNotExists([]byte(s.configBucket))
		if err != nil {
			return err
		}

		metaBucket, err := configBucket.CreateBucketIfNotExists([]byte(repository))
		if err != nil {
			return err
		}

		return metaBucket.Delete([]byte(name))
	})
}

// Open the database
func (s *Store) Open(readWrite bool) error {
	db, err := bbolt.Open(s.path, 0600, &bbolt.Options{ReadOnly: !readWrite})
	if err != nil {
		if readWrite {
			if err := os.Remove(s.path); err != nil {
				return fmt.Errorf("failed to remove corrupted database: %w", err)
			}
			if db, err = bbolt.Open(s.path, 0600, &bbolt.Options{ReadOnly: !readWrite}); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	s.db = db
	return nil
}

// NewStore returns a new configuration store
func NewStore(path string, readWrite bool, maxBucketSize int, configBucket string) (*Store, error) {
	s := &Store{
		path:          path,
		maxBucketSize: maxBucketSize,
		configBucket:  configBucket,
	}

	if err := s.Open(readWrite); err != nil {
		return nil, err
	}

	return s, nil
}
