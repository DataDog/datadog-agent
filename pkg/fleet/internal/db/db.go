// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package db provides a database to store information about packages
package db

import (
	"encoding/json"
	"fmt"
	"time"

	"go.etcd.io/bbolt"
)

var (
	bucketPackages = []byte("packages")
)

var (
	// ErrPackageNotFound is returned when a package is not found
	ErrPackageNotFound = fmt.Errorf("package not found")
)

// Package represents a package
type Package struct {
	Name    string
	Version string

	InstallerVersion string
}

// PackagesDB is a database that stores information about packages
type PackagesDB struct {
	db *bbolt.DB
}

type options struct {
	timeout time.Duration
}

// Option is a function that sets an option on a PackagesDB
type Option func(*options)

// WithTimeout sets the timeout for opening the database
func WithTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.timeout = timeout
	}
}

// New creates a new PackagesDB
func New(dbPath string, opts ...Option) (*PackagesDB, error) {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	db, err := bbolt.Open(dbPath, 0644, &bbolt.Options{
		Timeout:      o.timeout,
		FreelistType: bbolt.FreelistArrayType,
	})
	if err != nil {
		return nil, fmt.Errorf("could not open database: %w", err)
	}
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketPackages)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("could not create packages bucket: %w", err)
	}
	return &PackagesDB{
		db: db,
	}, nil
}

// Close closes the database
func (p *PackagesDB) Close() error {
	return p.db.Close()
}

// SetPackage sets a package
func (p *PackagesDB) SetPackage(pkg Package) error {
	err := p.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketPackages)
		if b == nil {
			return fmt.Errorf("bucket not found")
		}
		rawPkg, err := json.Marshal(&pkg)
		if err != nil {
			return fmt.Errorf("could not marshal package: %w", err)
		}
		return b.Put([]byte(pkg.Name), rawPkg)
	})
	if err != nil {
		return fmt.Errorf("could not set package: %w", err)
	}
	return nil
}

// DeletePackage deletes a package by name
func (p *PackagesDB) DeletePackage(name string) error {
	err := p.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketPackages)
		if b == nil {
			return fmt.Errorf("bucket not found")
		}
		return b.Delete([]byte(name))
	})
	if err != nil {
		return fmt.Errorf("could not delete package: %w", err)
	}
	return nil
}

// HasPackage checks if a package exists
func (p *PackagesDB) HasPackage(name string) (bool, error) {
	var hasPackage bool
	err := p.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketPackages)
		if b == nil {
			return fmt.Errorf("bucket not found")
		}
		v := b.Get([]byte(name))
		hasPackage = len(v) > 0
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("could not check if package exists: %w", err)
	}
	return hasPackage, nil
}

// GetPackage returns a package by name
func (p *PackagesDB) GetPackage(name string) (Package, error) {
	var pkg Package
	err := p.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketPackages)
		if b == nil {
			return fmt.Errorf("bucket not found")
		}
		v := b.Get([]byte(name))
		if len(v) == 0 {
			return ErrPackageNotFound
		}
		err := json.Unmarshal(v, &pkg)
		if err != nil {
			return fmt.Errorf("could not unmarshal package: %w", err)
		}
		return nil
	})
	if err != nil {
		return Package{}, fmt.Errorf("could not get package: %w", err)
	}
	return pkg, nil
}

// ListPackages returns a list of all packages
func (p *PackagesDB) ListPackages() ([]Package, error) {
	var pkgs []Package
	err := p.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketPackages)
		if b == nil {
			return fmt.Errorf("bucket not found")
		}
		return b.ForEach(func(k, v []byte) error {
			// support v0.0.7
			if len(v) == 0 {
				pkgs = append(pkgs, Package{Name: string(k)})
				return nil
			}
			var pkg Package
			err := json.Unmarshal(v, &pkg)
			if err != nil {
				return fmt.Errorf("could not unmarshal package: %w", err)
			}
			pkgs = append(pkgs, pkg)
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("could not list packages: %w", err)
	}
	return pkgs, nil
}
