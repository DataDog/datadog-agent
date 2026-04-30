// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rcserviceimpl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.etcd.io/bbolt"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

func rcFillFlare(svc rcservice.Component, runPath string) func(context.Context, flaretypes.FlareBuilder) error {
	return func(_ context.Context, fb flaretypes.FlareBuilder) error {
		if err := copyRemoteConfigDB(fb, runPath); err != nil {
			return err
		}

		state, err := svc.ConfigGetState()
		if err != nil {
			return fmt.Errorf("couldn't get the repositories state: %v", err)
		}

		var buf bytes.Buffer
		rcservice.PrintRemoteConfigStates(&buf, state, nil)
		return fb.AddFile("remote-config-state.log", buf.Bytes())
	}
}

func copyRemoteConfigDB(fb flaretypes.FlareBuilder, runPath string) error {
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
