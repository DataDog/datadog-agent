// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	"github.com/DataDog/datadog-agent/pkg/proto/msgpgo"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const metaBucket = "meta"
const metaFile = "meta.json"

type AgentMetadata struct {
	Version string `json:"version"`
}

func recreate(path string) (*bbolt.DB, error) {
	log.Infof("Clear remote configuration database")
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("could not check if rc db exists: (%s): %v", path, err)
	}
	if err == nil {
		err := os.Remove(path)
		if err != nil {
			return nil, fmt.Errorf("could not remote existing rc db (%s): %v", path, err)
		}
	}
	err = os.MkdirAll(filepath.Dir(path), 0700)
	if err != nil {
		return nil, fmt.Errorf("failed to create rc db dir: (%s): %v", path, err)
	}
	db, err := bbolt.Open(path, 0600, &bbolt.Options{})
	if err != nil {
		return nil, err
	}
	return db, addMetadata(db)
}

func addMetadata(db *bbolt.DB) error {
	return db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(metaBucket))
		if err != nil {
			return err
		}
		metaData, err := json.Marshal(AgentMetadata{
			Version: version.AgentVersion,
		})
		if err != nil {
			return err
		}
		return bucket.Put([]byte(metaFile), metaData)
	})
}

func openCacheDB(path string) (*bbolt.DB, error) {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{})
	if err != nil {
		return recreate(path)
	}

	metadata := new(AgentMetadata)
	err = db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(metaBucket))
		if bucket == nil {
			log.Infof("Missing meta bucket")
			return err
		}
		metadataBytes := bucket.Get([]byte(metaFile))
		if metadataBytes == nil {
			log.Infof("Missing meta file in meta bucket")
			return err
		}
		err = json.Unmarshal(metadataBytes, metadata)
		if err != nil {
			log.Infof("Invalid metadata")
			return err
		}
		return nil
	})
	if err != nil {
		_ = db.Close()
		return recreate(path)
	}

	if metadata.Version != version.AgentVersion {
		log.Infof("Different agent version detected")
		_ = db.Close()
		return recreate(path)
	}

	return db, nil
}

type remoteConfigAuthKeys struct {
	apiKey string

	rcKeySet bool
	rcKey    *msgpgo.RemoteConfigKey
}

func (k *remoteConfigAuthKeys) apiAuth() api.Auth {
	auth := api.Auth{
		ApiKey: k.apiKey,
	}
	if k.rcKeySet {
		auth.UseAppKey = true
		auth.AppKey = k.rcKey.AppKey
	}
	return auth
}

func getRemoteConfigAuthKeys(apiKey string, rcKey string) (remoteConfigAuthKeys, error) {
	if rcKey == "" {
		return remoteConfigAuthKeys{
			apiKey: apiKey,
		}, nil
	}
	rcKey = strings.TrimPrefix(rcKey, "DDRCM_")
	encoding := base32.StdEncoding.WithPadding(base32.NoPadding)
	rawKey, err := encoding.DecodeString(rcKey)
	if err != nil {
		return remoteConfigAuthKeys{}, err
	}
	var key msgpgo.RemoteConfigKey
	_, err = key.UnmarshalMsg(rawKey)
	if err != nil {
		return remoteConfigAuthKeys{}, err
	}
	if key.AppKey == "" || key.Datacenter == "" || key.OrgID == 0 {
		return remoteConfigAuthKeys{}, fmt.Errorf("invalid remote config key")
	}
	return remoteConfigAuthKeys{
		apiKey:   apiKey,
		rcKeySet: true,
		rcKey:    &key,
	}, nil
}

func buildLatestConfigsRequest(hostname string, traceAgentEnv string, orgUUID string, state uptane.TUFVersions, activeClients []*pbgo.Client, products map[data.Product]struct{}, newProducts map[data.Product]struct{}, lastUpdateErr error, clientState []byte) *pbgo.LatestConfigsRequest {
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

	lastUpdateErrString := ""
	if lastUpdateErr != nil {
		lastUpdateErrString = lastUpdateErr.Error()
	}
	return &pbgo.LatestConfigsRequest{
		Hostname:                     hostname,
		AgentVersion:                 version.AgentVersion,
		Products:                     data.ProductListToString(productsList),
		NewProducts:                  data.ProductListToString(newProductsList),
		CurrentConfigSnapshotVersion: state.ConfigSnapshot,
		CurrentConfigRootVersion:     state.ConfigRoot,
		CurrentDirectorRootVersion:   state.DirectorRoot,
		ActiveClients:                activeClients,
		BackendClientState:           clientState,
		HasError:                     lastUpdateErr != nil,
		Error:                        lastUpdateErrString,
		TraceAgentEnv:                traceAgentEnv,
		OrgUuid:                      orgUUID,
	}
}

type targetsCustom struct {
	OpaqueBackendState   []byte `json:"opaque_backend_state"`
	AgentRefreshInterval int64  `json:"agent_refresh_interval"`
}

func parseTargetsCustom(rawTargetsCustom []byte) (targetsCustom, error) {
	if len(rawTargetsCustom) == 0 {
		return targetsCustom{}, nil
	}
	var custom targetsCustom
	err := json.Unmarshal(rawTargetsCustom, &custom)
	if err != nil {
		return targetsCustom{}, err
	}
	return custom, nil
}
