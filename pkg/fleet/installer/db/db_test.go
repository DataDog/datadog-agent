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

func TestSetPackage(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	testPackage := Package{Name: "test", Version: "1.2.3", InstallerVersion: "4.5.6"}
	err := db.SetPackage(testPackage)
	assert.NoError(t, err)
	packages, err := db.ListPackages()
	assert.NoError(t, err)
	assert.Len(t, packages, 1)
	assert.Equal(t, testPackage, packages[0])
}

func TestDeletePackage(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	testPackage := Package{Name: "test", Version: "1.2.3", InstallerVersion: "4.5.6"}
	err := db.SetPackage(testPackage)
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

	testPackage1 := Package{Name: "test1", Version: "1.2.3", InstallerVersion: "4.5.6"}
	err = db.SetPackage(testPackage1)
	assert.NoError(t, err)
	testPackage2 := Package{Name: "test2", Version: "1.2.3", InstallerVersion: "4.5.6"}
	err = db.SetPackage(testPackage2)
	assert.NoError(t, err)
	packages, err = db.ListPackages()
	assert.NoError(t, err)
	assert.Len(t, packages, 2)
	assert.Contains(t, packages, testPackage1)
	assert.Contains(t, packages, testPackage2)
}

func TestGetPackage(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	testPackage := Package{Name: "test", Version: "1.2.3", InstallerVersion: "4.5.6"}
	err := db.SetPackage(testPackage)
	assert.NoError(t, err)
	p, err := db.GetPackage("test")
	assert.NoError(t, err)
	assert.Equal(t, testPackage, p)
}

func TestGetPackageNotFound(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	_, err := db.GetPackage("test")
	assert.ErrorIs(t, err, ErrPackageNotFound)
}

func TestHasPackage(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	testPackage := Package{Name: "test", Version: "1.2.3", InstallerVersion: "4.5.6"}
	err := db.SetPackage(testPackage)
	assert.NoError(t, err)
	has, err := db.HasPackage("test")
	assert.NoError(t, err)
	assert.True(t, has)
	has, err = db.HasPackage("test2")
	assert.NoError(t, err)
	assert.False(t, has)
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
