// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package flare contains helpers for including remote config data in agent flares.
package flare

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"go.etcd.io/bbolt"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

// CopyRemoteConfigDB copies the remote-config bbolt DB from runPath into the flare, hashing target
// bucket values to avoid including sensitive configuration payloads.
func CopyRemoteConfigDB(fb flaretypes.FlareBuilder, runPath string) error {
	dstPath, err := fb.PrepareFilePath("remote-config.db")
	if err != nil {
		return err
	}
	tempPath, err := fb.PrepareFilePath("remote-config.temp.db")
	if err != nil {
		return err
	}
	srcPath := filepath.Join(runPath, "remote-config.db")

	// Copy the DB to avoid bbolt lock contention and concurrent modifications.
	err = filesystem.CopyFileAll(srcPath, tempPath)
	defer os.Remove(tempPath)
	if err != nil {
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

func hashRCTargets(raw []byte) []byte {
	hash := sha256.Sum256(raw)
	return []byte(hex.EncodeToString(hash[:]))
}
