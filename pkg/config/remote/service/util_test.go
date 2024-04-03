// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package service

import (
	"bytes"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/proto/msgpgo"
)

const apiKey = "37d58c60b8ac337293ce2ca6b28b19eb"

func TestAuthKeys(t *testing.T) {
	tests := []struct {
		rcKey  string
		apiKey string
		err    bool
		output remoteConfigAuthKeys
	}{
		{apiKey: "37d58c60b8ac337293ce2ca6b28b19eb", rcKey: generateKey(t, 2, "datadoghq.com", "58d58c60b8ac337293ce2ca6b28b19eb"), output: remoteConfigAuthKeys{
			apiKey:   "37d58c60b8ac337293ce2ca6b28b19eb",
			rcKeySet: true,
			rcKey:    &msgpgo.RemoteConfigKey{AppKey: "58d58c60b8ac337293ce2ca6b28b19eb", OrgID: 2, Datacenter: "datadoghq.com"},
		}},
		{apiKey: "37d58c60b8ac337293ce2ca6b28b19eb", rcKey: "", output: remoteConfigAuthKeys{
			apiKey:   "37d58c60b8ac337293ce2ca6b28b19eb",
			rcKeySet: false,
		}},
		{rcKey: generateKey(t, 2, "datadoghq.com", ""), err: true},
		{rcKey: generateKey(t, 2, "", "app_Key"), err: true},
		{rcKey: generateKey(t, 0, "datadoghq.com", "app_Key"), err: true},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s|%s", test.apiKey, test.rcKey), func(tt *testing.T) {
			output, err := getRemoteConfigAuthKeys(test.apiKey, test.rcKey)
			if test.err {
				assert.Error(tt, err)
			} else {
				assert.Equal(tt, test.output, output)
				assert.NoError(tt, err)
			}
		})
	}
}

func generateKey(t *testing.T, orgID int64, datacenter string, appKey string) string {
	key := msgpgo.RemoteConfigKey{
		AppKey:     appKey,
		OrgID:      orgID,
		Datacenter: datacenter,
	}
	rawKey, err := key.MarshalMsg(nil)
	if err != nil {
		t.Fatal(err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(rawKey)
}

func addData(db *bbolt.DB) error {
	return db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucket([]byte("test"))
		if err != nil {
			return err
		}

		return bucket.Put([]byte("test"), []byte("test"))
	})
}

func checkData(db *bbolt.DB) error {
	return db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("test"))
		if bucket == nil {
			return fmt.Errorf("Bucket not present")
		}

		data := bucket.Get([]byte("test"))
		if !bytes.Equal(data, []byte("test")) {
			return fmt.Errorf("Invalid test data")
		}
		return nil
	})
}

func getMetadata(db *bbolt.DB) (*AgentMetadata, error) {
	tx, err := db.Begin(false)
	defer tx.Rollback()
	if err != nil {
		return nil, err
	}
	bucket := tx.Bucket([]byte(metaBucket))
	if bucket == nil {
		return nil, fmt.Errorf("No bucket")
	}
	metaBytes := bucket.Get([]byte(metaFile))
	if metaBytes == nil {
		return nil, fmt.Errorf("No meta file")
	}
	metadata := new(AgentMetadata)
	err = json.Unmarshal(metaBytes, metadata)
	return metadata, err
}

func TestRemoteConfigNewDB(t *testing.T) {
	dir, err := os.MkdirTemp("", "remote-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// should add the version to newly created databases
	db, err := openCacheDB(filepath.Join(dir, "remote-config.db"), "9.9.9", apiKey)
	require.NoError(t, err)
	defer db.Close()

	metadata, err := getMetadata(db)
	require.NoError(t, err)

	assert.Equal(t, agentVersion, metadata.Version)
}

func TestRemoteConfigChangedAPIKey(t *testing.T) {
	dir, err := os.MkdirTemp("", "remote-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// should add the version to newly created databases
	db0, err := openCacheDB(filepath.Join(dir, "remote-config.db"), "9.9.9", apiKey)
	require.NoError(t, err)
	defer db0.Close()
	metadata0, err := getMetadata(db0)
	require.NoError(t, err)
	db0.Close()

	db1, err := openCacheDB(filepath.Join(dir, "remote-config.db"), "9.9.9", apiKey+"-new")
	require.NoError(t, err)
	defer db1.Close()
	metadata1, err := getMetadata(db1)
	require.NoError(t, err)

	require.NotEqual(t, metadata0.APIKeyHash, metadata1.APIKeyHash)
	require.NotEqual(t, metadata0.CreationTime, metadata1.CreationTime)
}

func TestRemoteConfigReopenNoVersionChange(t *testing.T) {
	dir, err := os.MkdirTemp("", "remote-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// should add the version to newly created databases
	db, err := openCacheDB(filepath.Join(dir, "remote-config.db"), agentVersion, apiKey)
	require.NoError(t, err)

	metadata, err := getMetadata(db)
	require.NoError(t, err)

	assert.Equal(t, agentVersion, metadata.Version)
	require.NoError(t, addData(db))
	require.NoError(t, db.Close())

	db, err = openCacheDB(filepath.Join(dir, "remote-config.db"), agentVersion, apiKey)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, checkData(db))
}

func TestRemoteConfigOldDB(t *testing.T) {
	dir, err := os.MkdirTemp("", "remote-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	dbPath := filepath.Join(dir, "remote-config.db")

	// create database with current version
	db, err := openCacheDB(dbPath, agentVersion, apiKey)
	require.NoError(t, err)

	require.NoError(t, addData(db))

	// set it to another version
	metadata, err := json.Marshal(AgentMetadata{Version: "old-version"})
	require.NoError(t, err)
	err = db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(metaBucket))
		return bucket.Put([]byte(metaFile), []byte(metadata))
	})
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// reopen database
	db, err = openCacheDB(dbPath, agentVersion, apiKey)
	require.NoError(t, err)

	// check version after the database opens
	parsedMeta, err := getMetadata(db)
	require.NoError(t, err)

	assert.Equal(t, agentVersion, parsedMeta.Version)
	assert.Error(t, checkData(db))
}
