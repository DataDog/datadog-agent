// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package trivy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBoltDB_Close(t *testing.T) {
	db, err := NewBoltDB(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, db.Close())
	require.Error(t, db.Store("key", []byte("value")))
}

func TestBoltDB_Clear(t *testing.T) {
	db, err := NewBoltDB(t.TempDir())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	require.NoError(t, db.Store("key", []byte("value")))
	require.NoError(t, db.Clear())

	require.Error(t, db.Store("key", []byte("value")))
	_, err = db.Get("key")
	require.Error(t, err)
}

func TestBoltDB_StoreAndGet(t *testing.T) {
	db, err := NewBoltDB(t.TempDir())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()
	key := "key"
	value := []byte("value")
	require.NoError(t, db.Store(key, value))

	retrieved, err := db.Get(key)
	require.NoError(t, err)
	require.Equal(t, value, retrieved)
}

func TestBoltDB_Delete(t *testing.T) {
	db, err := NewBoltDB(t.TempDir())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	require.NoError(t, db.Store("key1", []byte("value1")))
	require.NoError(t, db.Store("key2", []byte("value2")))
	require.NoError(t, db.Store("key3", []byte("value3")))

	deletedValues := 0
	keysToDelete := []string{"key1", "key3"}
	err = db.Delete(keysToDelete, func(_ string, _ []byte) error {
		deletedValues += 1
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, deletedValues)

	value1, err := db.Get("key1")
	require.NoError(t, err)
	require.Equal(t, []byte(nil), value1)

	value2, err := db.Get("key2")
	require.NoError(t, err)
	require.Equal(t, []byte("value2"), value2)

	value3, err := db.Get("key3")
	require.NoError(t, err)
	require.Equal(t, []byte(nil), value3)
}

func TestBoltDB_ForEach(t *testing.T) {
	db, err := NewBoltDB(t.TempDir())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	testData := map[string][]byte{
		"key1": []byte("value1"),
		"key2": []byte("value2"),
	}

	for key, value := range testData {
		require.NoError(t, db.Store(key, value))
	}

	var receivedKeys []string
	var receivedValues [][]byte

	err = db.ForEach(func(key string, value []byte) error {
		receivedKeys = append(receivedKeys, key)
		receivedValues = append(receivedValues, value)
		return nil
	})
	require.NoError(t, err)

	require.Equal(t, len(receivedKeys), len(testData))
	require.Equal(t, len(receivedValues), len(testData))

	for i, key := range receivedKeys {
		value, ok := testData[key]
		require.True(t, ok)
		require.Equal(t, value, receivedValues[i])
	}
}
