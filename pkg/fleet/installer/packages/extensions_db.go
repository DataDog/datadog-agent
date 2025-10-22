// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"encoding/json"
	"fmt"

	"go.etcd.io/bbolt"
)

var (
	bucketExtensions   = []byte("extensions")
	errPackageNotFound = fmt.Errorf("package not found")
)

type dbPackage struct {
	Name       string              `json:"name"`
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

// GetPackage returns a package by name
func (p *extensionsDB) GetPackage(name string, experiment bool) (dbPackage, error) {
	var pkg dbPackage
	err := p.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketExtensions)
		if b == nil {
			return fmt.Errorf("bucket not found")
		}
		v := b.Get([]byte(fmt.Sprintf("%s-%t", name, experiment)))
		if len(v) == 0 {
			return errPackageNotFound
		}
		err := json.Unmarshal(v, &pkg)
		if err != nil {
			return fmt.Errorf("could not unmarshal package: %w", err)
		}
		return nil
	})
	if err != nil {
		return dbPackage{}, fmt.Errorf("could not get package: %w", err)
	}
	return pkg, nil
}

// SetPackage sets a package
func (p *extensionsDB) SetPackage(pkg dbPackage, experiment bool) error {
	err := p.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketExtensions)
		if b == nil {
			return fmt.Errorf("bucket not found")
		}
		rawPkg, err := json.Marshal(&pkg)
		if err != nil {
			return fmt.Errorf("could not marshal package: %w", err)
		}
		return b.Put([]byte(fmt.Sprintf("%s-%t", pkg.Name, experiment)), rawPkg)
	})
	if err != nil {
		return fmt.Errorf("could not set package: %w", err)
	}
	return nil
}

// RemovePackage removes a package
func (p *extensionsDB) RemovePackage(name string, experiment bool) error {
	err := p.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketExtensions)
		if b == nil {
			return fmt.Errorf("bucket not found")
		}
		return b.Delete([]byte(fmt.Sprintf("%s-%t", name, experiment)))
	})
	if err != nil {
		return fmt.Errorf("could not remove package: %w", err)
	}
	return nil
}

// Promote promotes a key from experiment to stable
func (p *extensionsDB) Promote(name string) error {
	content, err := p.GetPackage(name, true)
	if err != nil {
		return fmt.Errorf("could not get package: %w", err)
	}
	err = p.SetPackage(content, false)
	if err != nil {
		return fmt.Errorf("could not set package: %w", err)
	}
	err = p.RemovePackage(name, true)
	if err != nil {
		return fmt.Errorf("could not remove package: %w", err)
	}
	return nil
}
