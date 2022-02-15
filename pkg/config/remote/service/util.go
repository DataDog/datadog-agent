// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"encoding/base32"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	"github.com/DataDog/datadog-agent/pkg/proto/msgpgo"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/version"
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

func parseRemoteConfigKey(serializedKey string) (*msgpgo.RemoteConfigKey, error) {
	encoding := base32.StdEncoding.WithPadding(base32.NoPadding)
	rawKey, err := encoding.DecodeString(serializedKey)
	if err != nil {
		return nil, err
	}
	var key msgpgo.RemoteConfigKey
	_, err = key.UnmarshalMsg(rawKey)
	if err != nil {
		return nil, err
	}
	if key.AppKey == "" || key.Datacenter == "" || key.OrgID == 0 {
		return nil, fmt.Errorf("invalid remote config key")
	}
	return &key, nil
}

func buildLatestConfigsRequest(hostname string, state uptane.State, activeClients []*pbgo.Client, products map[data.Product]struct{}, newProducts map[data.Product]struct{}) *pbgo.LatestConfigsRequest {
	productsList := make([]data.Product, len(products))
	i := 0
	for k := range products {
		productsList[i] = k
		i++
	}
	newProductsList := make([]data.Product, len(newProducts))
	i = 0
	for k := range newProducts {
		newProductsList[i] = k
		i++
	}

	return &pbgo.LatestConfigsRequest{
		Hostname:                     hostname,
		AgentVersion:                 version.AgentVersion,
		Products:                     data.ProductListToString(productsList),
		NewProducts:                  data.ProductListToString(newProductsList),
		CurrentConfigSnapshotVersion: state.ConfigSnapshotVersion,
		CurrentConfigRootVersion:     state.ConfigRootVersion,
		CurrentDirectorRootVersion:   state.DirectorRootVersion,
		ActiveClients:                activeClients,
	}
}
