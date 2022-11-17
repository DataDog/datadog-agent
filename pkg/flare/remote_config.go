// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flare

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"go.etcd.io/bbolt"
)

func hashRCTargets(raw []byte) []byte {
	hash := sha256.Sum256(raw)
	// Convert it to readable hex
	s := hex.EncodeToString(hash[:])

	return []byte(s)
}

func zipRemoteConfigDB(tempDir, hostname string) error {
	dstPath := filepath.Join(tempDir, hostname, "remote-config.db")
	tempPath := filepath.Join(tempDir, hostname, "remote-config.temp.db")
	srcPath := filepath.Join(config.Datadog.GetString("run_path"), "remote-config.db")

	// Copies the db so it avoids bbolt from being locked
	// Also avoid concurrent modifications
	err := util.CopyFileAll(srcPath, tempPath)
	// Delete the db at the end to avoid having target files content
	defer os.Remove(tempPath)
	if err != nil {
		// Prevent from having a clear db here
		return err
	}

	tempDB, err := bbolt.Open(tempPath, 0400, &bbolt.Options{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tempDB.Close()
	dstDB, err := bbolt.Open(dstPath, 0600, &bbolt.Options{})
	if err != nil {
		return err
	}
	defer dstDB.Close()

	return tempDB.View(func(tempTx *bbolt.Tx) error {
		return tempTx.ForEach(func(bucketName []byte, tempBucket *bbolt.Bucket) error {
			return dstDB.Update(func(dstTx *bbolt.Tx) error {
				dstBucket, err := dstTx.CreateBucket(bucketName)
				if err != nil {
					return err
				}
				return tempBucket.ForEach(func(k, v []byte) error {
					if strings.HasSuffix(string(bucketName), "_targets") {
						return dstBucket.Put(k, hashRCTargets(v))
					}
					return dstBucket.Put(k, v)
				})
			})
		})
	})
}
