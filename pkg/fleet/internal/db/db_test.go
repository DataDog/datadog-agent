// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.etcd.io/bbolt"
)

func newTestDB(t *testing.T) *PackagesDB {
	db, err := New(filepath.Join(t.TempDir(), "test.db"))
	assert.NoError(t, err)
	return db
}

func TestCreatePackage(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	err := db.CreatePackage("test")
	assert.NoError(t, err)
	packages, err := db.ListPackages()
	assert.NoError(t, err)
	assert.Len(t, packages, 1)
	assert.Equal(t, "test", packages[0])
}

func TestDeletePackage(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	err := db.CreatePackage("test")
	assert.NoError(t, err)
	err = db.DeletePackage("test")
	assert.NoError(t, err)
	packages, err := db.ListPackages()
	assert.NoError(t, err)
	assert.Len(t, packages, 0)
}

func TestListPackages(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	packages, err := db.ListPackages()
	assert.NoError(t, err)
	assert.Len(t, packages, 0)

	err = db.CreatePackage("test1")
	assert.NoError(t, err)
	err = db.CreatePackage("test2")
	assert.NoError(t, err)
	packages, err = db.ListPackages()
	assert.NoError(t, err)
	assert.Len(t, packages, 2)
	assert.Contains(t, packages, "test1")
	assert.Contains(t, packages, "test2")
}

func TestTimeout(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "test.db")
	dbLock, err := New(dbFile)
	assert.NoError(t, err)
	defer dbLock.Close()

	before := time.Now()
	_, err = New(dbFile, WithTimeout(time.Second))
	assert.ErrorIs(t, err, bbolt.ErrTimeout)
	assert.GreaterOrEqual(t, time.Since(before), time.Second-100*time.Millisecond) // bbolt timeout can be shorter by up to 50ms
}
