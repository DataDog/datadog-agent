// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/go-tuf/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

const apiKey = "37d58c60b8ac337293ce2ca6b28b19eb"
const rcURL = "dd-rc-url"
const agentVersion = "9.9.9"

func TestParseMetaPath(t *testing.T) {
	tests := []struct {
		input  string
		err    bool
		output metaPath
	}{
		{input: "1.root.json", err: false, output: metaPath{role: "root", version: 1, versionSet: true}},
		{input: "5.timestamp.json", err: false, output: metaPath{role: "timestamp", version: 5, versionSet: true}},
		{input: "timestamp.json", err: false, output: metaPath{role: "timestamp", version: 0, versionSet: false}},
		{input: ".timestamp.json", err: true, output: metaPath{}},
		{input: "5.timestamp.", err: true, output: metaPath{}},
		{input: "5..json", err: true, output: metaPath{}},
		{input: "5.timestamp.ext", err: true, output: metaPath{}},
		{input: "..", err: true, output: metaPath{}},
		{input: "", err: true, output: metaPath{}},
	}
	for _, test := range tests {
		t.Run(test.input, func(tt *testing.T) {
			output, err := parseMetaPath(test.input)
			if test.err {
				assert.Error(tt, err)
			} else {
				assert.Equal(tt, test.output, output)
				assert.NoError(tt, err)
			}
		})
	}
}

func serializeTestMeta(meta interface{}) json.RawMessage {
	serializedMeta, _ := json.Marshal(meta)
	signedMeta := data.Signed{Signed: serializedMeta, Signatures: []data.Signature{}}
	serializedSignedMeta, _ := json.Marshal(signedMeta)
	return serializedSignedMeta
}

func TestMetaFields(t *testing.T) {
	root := data.NewRoot()
	root.Version = 1
	timestamp := data.NewTimestamp()
	timestamp.Version = 2
	snapshot := data.NewSnapshot()
	snapshot.Version = 3
	targets := data.NewTargets()
	targets.Version = 4

	tests := []struct {
		name      string
		input     json.RawMessage
		err       bool
		version   uint64
		timestamp time.Time
	}{
		{name: "root", input: serializeTestMeta(root), err: false, version: 1, timestamp: root.Expires},
		{name: "timestamp", input: serializeTestMeta(timestamp), err: false, version: 2, timestamp: timestamp.Expires},
		{name: "snapshot", input: serializeTestMeta(snapshot), err: false, version: 3, timestamp: snapshot.Expires},
		{name: "targets", input: serializeTestMeta(targets), err: false, version: 4, timestamp: targets.Expires},
		{name: "invalid", input: json.RawMessage(`{}`), err: true, version: 0, timestamp: time.Time{}},
		{name: "invalid2", input: json.RawMessage(``), err: true, version: 0, timestamp: time.Time{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			output, err := unsafeMetaVersion(test.input)
			if test.err {
				assert.Error(tt, err)
			} else {
				assert.Equal(tt, test.version, output)
				assert.NoError(tt, err)
			}
			tsOutput, err := unsafeMetaExpires(test.input)
			if test.err {
				assert.Error(tt, err)
			} else {
				assert.Equal(tt, test.timestamp, tsOutput)
				assert.NoError(tt, err)
			}
		})
	}
}

func TestMetaCustom(t *testing.T) {
	root := data.NewRoot()
	var custom1 = json.RawMessage([]byte("1"))
	root.Custom = &custom1
	timestamp := data.NewTimestamp()
	var custom2 = json.RawMessage([]byte("2"))
	timestamp.Custom = &custom2
	snapshot := data.NewSnapshot()
	targets := data.NewTargets()
	var custom4 = json.RawMessage([]byte(`{"a":4}`))
	targets.Custom = &custom4

	tests := []struct {
		name   string
		input  json.RawMessage
		err    bool
		output []byte
	}{
		{name: "root", input: serializeTestMeta(root), err: false, output: []byte("1")},
		{name: "timestamp", input: serializeTestMeta(timestamp), err: false, output: []byte("2")},
		{name: "snapshot", input: serializeTestMeta(snapshot), err: false, output: nil},
		{name: "targets", input: serializeTestMeta(targets), err: false, output: []byte(`{"a":4}`)},
		{name: "invalid", input: json.RawMessage(`{}`), err: true, output: nil},
		{name: "invalid2", input: json.RawMessage(``), err: true, output: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			output, err := unsafeMetaCustom(test.input)
			if test.err {
				assert.Error(tt, err)
			} else {
				assert.Equal(tt, test.output, output)
				assert.NoError(tt, err)
			}
		})
	}
}

func TestTrimHashTargetPath(t *testing.T) {
	tests := []struct {
		input  string
		output string
	}{
		{input: "2/APM_SAMPLING/479520024efb527de298760ad09b8d89561d81c31fcff476e9b034a110948b64.1", output: "2/APM_SAMPLING/1"},
		{input: "2/APM_SAMPLING/target2", output: "2/APM_SAMPLING/target2"},
	}
	for _, test := range tests {
		t.Run(test.input, func(tt *testing.T) {
			output := trimHashTargetPath(test.input)
			assert.Equal(tt, test.output, output)
		})
	}
}

func getBucketMetadata(db *bbolt.DB) (*AgentMetadata, error) {
	tx, err := db.Begin(false)
	defer tx.Rollback()
	if err != nil {
		return nil, err
	}
	bucket := tx.Bucket([]byte(metaBucket))
	if bucket == nil {
		return nil, errors.New("No bucket")
	}
	metaBytes := bucket.Get([]byte(metaFile))
	if metaBytes == nil {
		return nil, errors.New("No meta file")
	}
	metadata := new(AgentMetadata)
	err = json.Unmarshal(metaBytes, metadata)
	return metadata, err
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
			return errors.New("Bucket not present")
		}

		data := bucket.Get([]byte("test"))
		if !bytes.Equal(data, []byte("test")) {
			return errors.New("Invalid test data")
		}
		return nil
	})
}

func TestRemoteConfigNewDB(t *testing.T) {
	dir, err := os.MkdirTemp("", "remote-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// should add the version to newly created databases
	db, err := openCacheDB(filepath.Join(dir, "remote-config.db"), "9.9.9", apiKey, rcURL)
	require.NoError(t, err)
	defer db.Close()

	metadata, err := getBucketMetadata(db)
	require.NoError(t, err)

	assert.Equal(t, agentVersion, metadata.Version)
}

func TestRemoteConfigChangedAPIKey(t *testing.T) {
	dir, err := os.MkdirTemp("", "remote-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// should add the version to newly created databases
	db0, err := openCacheDB(filepath.Join(dir, "remote-config.db"), "9.9.9", apiKey, rcURL)
	require.NoError(t, err)
	defer db0.Close()
	metadata0, err := getBucketMetadata(db0)
	require.NoError(t, err)
	db0.Close()

	db1, err := openCacheDB(filepath.Join(dir, "remote-config.db"), "9.9.9", apiKey+"-new", rcURL)
	require.NoError(t, err)
	defer db1.Close()
	metadata1, err := getBucketMetadata(db1)
	require.NoError(t, err)

	require.NotEqual(t, metadata0.APIKeyHash, metadata1.APIKeyHash)
}

func TestRemoteConfigReopenNoVersionChange(t *testing.T) {
	dir, err := os.MkdirTemp("", "remote-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// should add the version to newly created databases
	db, err := openCacheDB(filepath.Join(dir, "remote-config.db"), agentVersion, apiKey, rcURL)
	require.NoError(t, err)

	metadata, err := getBucketMetadata(db)
	require.NoError(t, err)

	assert.Equal(t, agentVersion, metadata.Version)
	require.NoError(t, addData(db))
	require.NoError(t, db.Close())

	db, err = openCacheDB(filepath.Join(dir, "remote-config.db"), agentVersion, apiKey, rcURL)
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
	db, err := openCacheDB(dbPath, agentVersion, apiKey, rcURL)
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
	db, err = openCacheDB(dbPath, agentVersion, apiKey, rcURL)
	require.NoError(t, err)

	// check version after the database opens
	parsedMeta, err := getBucketMetadata(db)
	require.NoError(t, err)

	assert.Equal(t, agentVersion, parsedMeta.Version)
	assert.Error(t, checkData(db))
}

func TestRemoteConfigChangedURL(t *testing.T) {
	dir, err := os.MkdirTemp("", "remote-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// should add the version to newly created databases
	db0, err := openCacheDB(filepath.Join(dir, "remote-config.db"), "9.9.9", apiKey, rcURL)
	require.NoError(t, err)
	defer db0.Close()
	metadata0, err := getBucketMetadata(db0)
	require.NoError(t, err)
	db0.Close()

	db1, err := openCacheDB(filepath.Join(dir, "remote-config.db"), "9.9.9", apiKey, rcURL+"-new")
	require.NoError(t, err)
	defer db1.Close()
	metadata1, err := getBucketMetadata(db1)
	require.NoError(t, err)

	require.NotEqual(t, metadata0.URL, metadata1.URL)
}
