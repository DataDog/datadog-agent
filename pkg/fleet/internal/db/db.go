// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package db provides a database to store information about packages
package db

import (
	"fmt"
	"time"

	"go.etcd.io/bbolt"
)

var (
	bucketPackages = []byte("packages")
)

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

// CreatePackage sets a package
func (p *PackagesDB) CreatePackage(name string) error {
	err := p.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketPackages)
		if b == nil {
			return fmt.Errorf("bucket not found")
		}
		return b.Put([]byte(name), []byte{})
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

// ListPackages returns a list of all packages
func (p *PackagesDB) ListPackages() ([]string, error) {
	var pkgs []string
	err := p.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketPackages)
		if b == nil {
			return fmt.Errorf("bucket not found")
		}
		return b.ForEach(func(k, v []byte) error {
			pkgs = append(pkgs, string(k))
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("could not list packages: %w", err)
	}
	return pkgs, nil
}
