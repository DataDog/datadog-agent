// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"encoding/base32"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/gogo/protobuf/proto"
	"go.etcd.io/bbolt"
)

func openCacheDB(path string) (*bbolt.DB, error) {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{})
	if err != nil {
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("failed to remove corrupted database: %w", err)
		}
		if db, err = bbolt.Open(path, 0600, &bbolt.Options{}); err != nil {
			return nil, err
		}
	}
	return db, nil
}

func parseRemoteConfigKey(serializedKey string) (*pbgo.RemoteConfigKey, error) {
	encoding := base32.StdEncoding.WithPadding(base32.NoPadding)
	rawKey, err := encoding.DecodeString(serializedKey)
	if err != nil {
		return nil, err
	}
	var key pbgo.RemoteConfigKey
	err = proto.Unmarshal(rawKey, &key)
	if err != nil {
		return nil, err
	}
	if key.AppKey == "" || key.Datacenter == "" || key.OrgId == 0 {
		return nil, fmt.Errorf("invalid remote config key")
	}
	return &key, nil
}
