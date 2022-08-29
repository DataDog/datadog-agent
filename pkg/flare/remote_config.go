// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.etcd.io/bbolt"
)

func hashedRCTargets(raw []byte) []byte {
	hash := sha256.Sum256(raw)
	return hash[:]
}

func zipRemoteConfigDB(tempDir, hostname string) error {
	dstPath := filepath.Join(tempDir, hostname, "remote-config.db")
	srcPath := filepath.Join(config.Datadog.GetString("run_path"), "remote-config.db")

	err := util.CopyFileAll(srcPath, dstPath)
	if err != nil {
		// Prevent from having a clear db here
		os.Remove(dstPath)
		return err
	}

	dstDB, err := bbolt.Open(dstPath, 0600, &bbolt.Options{})
	if err != nil {
		os.Remove(dstPath)
		return err
	}
	defer dstDB.Close()

	err = dstDB.Update(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bbolt.Bucket) error {
			if strings.HasSuffix(string(name), "_targets") {
				log.Errorf("replacing targets")
				newBucket, err := tx.CreateBucket([]byte(fmt.Sprintf("%s_hashed", string(name))))
				if err != nil {
					return err
				}
				cursor := b.Cursor()
				for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
					if err := newBucket.Put(k, hashedRCTargets(v)); err != nil {
						return err
					}
				}
				if err := b.DeleteBucket(name); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		// Prevent from having a clear db here
		os.Remove(dstPath)
		return err
	}

	return nil
}
