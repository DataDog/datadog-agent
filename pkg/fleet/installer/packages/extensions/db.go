// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package extensions

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"
)

var (
	bucketExtensions   = []byte("extensions")
	errPackageNotFound = errors.New("package not found")
)

type dbPackage struct {
	Name       string              `json:"pkg"`
	Version    string              `json:"version"`
	Extensions map[string]struct{} `json:"extensions"`
}

// extensionsDB is a database that stores information about extensions.
type extensionsDB struct {
	db *bbolt.DB
}

// newExtensionsDB creates a new extensionsDB. It acts as a lock for extensions operations.
func newExtensionsDB(dbPath string) (*extensionsDB, error) {
	db, err := bbolt.Open(dbPath, 0644, &bbolt.Options{
		FreelistType: bbolt.FreelistArrayType,
	})
	if err != nil {
		return nil, fmt.Errorf("could not open database: %w", err)
	}
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketExtensions)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("could not create extensions bucket: %w", err)
	}
	return &extensionsDB{
		db: db,
	}, nil
}

// Close closes the database
func (p *extensionsDB) Close() error {
	return p.db.Close()
}

// GetPackage returns a package by pkg
func (p *extensionsDB) GetPackage(pkg string, isExperiment bool) (dbPackage, error) {
	var dbPkg dbPackage
	err := p.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketExtensions)
		if b == nil {
			return errors.New("bucket not found")
		}
		v := b.Get(getKey(pkg, isExperiment))
		if len(v) == 0 {
			return errPackageNotFound
		}
		err := json.Unmarshal(v, &dbPkg)
		if err != nil {
			return fmt.Errorf("could not unmarshal package: %w", err)
		}
		return nil
	})
	if err != nil {
		return dbPackage{}, fmt.Errorf("could not get package: %w", err)
	}
	return dbPkg, nil
}

// SetPackage sets a package
func (p *extensionsDB) SetPackage(dbPkg dbPackage, isExperiment bool) error {
	err := p.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketExtensions)
		if b == nil {
			return errors.New("bucket not found")
		}
		rawPkg, err := json.Marshal(&dbPkg)
		if err != nil {
			return fmt.Errorf("could not marshal package: %w", err)
		}
		return b.Put(getKey(dbPkg.Name, isExperiment), rawPkg)
	})
	if err != nil {
		return fmt.Errorf("could not set package: %w", err)
	}
	return nil
}

// RemovePackage removes a package
func (p *extensionsDB) RemovePackage(pkg string, isExperiment bool) error {
	err := p.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketExtensions)
		if b == nil {
			return errors.New("bucket not found")
		}
		return b.Delete(getKey(pkg, isExperiment))
	})
	if err != nil {
		return fmt.Errorf("could not remove package: %w", err)
	}
	return nil
}

// Promote promotes a key from isExperiment to stable
func (p *extensionsDB) PromotePackage(pkg string) error {
	content, err := p.GetPackage(pkg, true)
	if err != nil {
		return fmt.Errorf("could not get package: %w", err)
	}
	err = p.SetPackage(content, false)
	if err != nil {
		return fmt.Errorf("could not set package: %w", err)
	}
	err = p.RemovePackage(pkg, true)
	if err != nil {
		return fmt.Errorf("could not remove package: %w", err)
	}
	return nil
}

func (p *extensionsDB) SetPackageVersion(pkg string, version string, isExperiment bool) error {
	err := p.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketExtensions)
		if b == nil {
			return errors.New("bucket not found")
		}
		dbPkg := dbPackage{
			Name:    pkg,
			Version: version,
		}
		rawPkg, err := json.Marshal(&dbPkg)
		if err != nil {
			return fmt.Errorf("could not marshal package: %w", err)
		}
		return b.Put(getKey(pkg, isExperiment), rawPkg)
	})
	if err != nil {
		return fmt.Errorf("could not set package version: %w", err)
	}
	return nil
}

func getKey(pkg string, isExperiment bool) []byte {
	if isExperiment {
		return []byte(pkg + "-exp")
	}
	return []byte(pkg)
}
