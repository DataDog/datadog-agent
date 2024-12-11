// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	"github.com/DataDog/datadog-agent/pkg/proto/msgpgo"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

const metaBucket = "meta"
const metaFile = "meta.json"
const databaseLockTimeout = time.Second

// AgentMetadata is data stored in bolt DB to determine whether or not
// the agent has changed and the RC cache should be cleared
type AgentMetadata struct {
	Version      string    `json:"version"`
	APIKeyHash   string    `json:"api-key-hash"`
	CreationTime time.Time `json:"creation-time"`
}

// hashAPIKey hashes the API key to avoid storing it in plain text using SHA256
func hashAPIKey(apiKey string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(apiKey)))
}

func recreate(path string, agentVersion string, apiKeyHash string) (*bbolt.DB, error) {
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
	db, err := bbolt.Open(path, 0600, &bbolt.Options{
		Timeout: databaseLockTimeout,
	})
	if err != nil {
		if errors.Is(err, bbolt.ErrTimeout) {
			return nil, fmt.Errorf("rc db is locked. Please check if another instance of the agent is running and using the same `run_path` parameter")
		}
		return nil, err
	}
	return db, addMetadata(db, agentVersion, apiKeyHash)
}

func addMetadata(db *bbolt.DB, agentVersion string, apiKeyHash string) error {
	return db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(metaBucket))
		if err != nil {
			return err
		}
		metaData, err := json.Marshal(AgentMetadata{
			Version:      agentVersion,
			APIKeyHash:   apiKeyHash,
			CreationTime: time.Now(),
		})
		if err != nil {
			return err
		}
		return bucket.Put([]byte(metaFile), metaData)
	})
}

func openCacheDB(path string, agentVersion string, apiKey string) (*bbolt.DB, error) {
	apiKeyHash := hashAPIKey(apiKey)

	db, err := bbolt.Open(path, 0600, &bbolt.Options{
		Timeout: databaseLockTimeout,
	})
	if err != nil {
		if errors.Is(err, bbolt.ErrTimeout) {
			return nil, fmt.Errorf("rc db is locked. Please check if another instance of the agent is running and using the same `run_path` parameter")
		}
		return recreate(path, agentVersion, apiKeyHash)
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
		return recreate(path, agentVersion, apiKeyHash)
	}

	if metadata.Version != agentVersion || metadata.APIKeyHash != apiKeyHash {
		log.Infof("Different agent version or API Key detected")
		_ = db.Close()
		return recreate(path, agentVersion, apiKeyHash)
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
		APIKey: k.apiKey,
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

func buildLatestConfigsRequest(hostname string, agentVersion string, tags []string, traceAgentEnv string, orgUUID string, state uptane.TUFVersions, activeClients []*pbgo.Client, products map[data.Product]struct{}, newProducts map[data.Product]struct{}, lastUpdateErr error, clientState []byte) *pbgo.LatestConfigsRequest {
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
		AgentUuid:                    uuid.GetUUID(),
		AgentVersion:                 agentVersion,
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
		Tags:                         tags,
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
