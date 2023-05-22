// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build podman

package podman

// The podman go package brings a lot of dependencies because it includes the
// server and the client. Some of those dependencies are problematic for us. For
// example, it depends on a more recent version of `go-systemd` than the one we
// are using.
//
// To avoid bringing unnecessary dependencies, we have decided to write our own
// simple client that queries the BoltDB used by Podman directly. This is
// feasible because we just need a very small subset of the features offered by
// the Podman package. More specifically, we just need to retrieve the exiting
// containers from the DB.
//
// The functions in this file have been copied from
// https://github.com/containers/podman/blob/v3.4.1/libpod/boltdb_state_internal.go
// The code has been adapted a bit to our needs. The only functions of that file
// that we need are AllContainers() and the helpers that it uses.
//
// This code could break in future versions of Podman. This has been tried with
// v3.4.1.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ctrName     = "ctr"
	allCtrsName = "all-ctrs"
	configName  = "config"
	stateName   = "state"
	openTimeout = 30 * time.Second
)

var (
	ctrBkt     = []byte(ctrName)
	allCtrsBkt = []byte(allCtrsName)
	configKey  = []byte(configName)
	stateKey   = []byte(stateName)
)

// DBClient is a client for the podman's state database
type DBClient struct {
	DBPath string
}

// NewDBClient returns a DB client that uses the DB stored in dbPath.
func NewDBClient(dbPath string) *DBClient {
	return &DBClient{
		DBPath: dbPath,
	}
}

// GetAllContainers returns all the containers present in the DB
func (client *DBClient) GetAllContainers() ([]Container, error) {
	var res []Container

	db, err := client.getDBCon()
	if err != nil {
		return nil, err
	}
	defer func() {
		if errClose := db.Close(); errClose != nil {
			log.Warnf("failed to close libpod db: %q", err)
		}
	}()

	err = db.View(func(tx *bolt.Tx) error {
		allCtrsBucket, err := getAllCtrsBucket(tx)
		if err != nil {
			return err
		}

		ctrBucket, err := getCtrBucket(tx)
		if err != nil {
			return err
		}

		return allCtrsBucket.ForEach(func(id, name []byte) error {
			// If performance becomes an issue, this check can be
			// removed. But the error messages that come back will
			// be much less helpful.
			ctrExists := ctrBucket.Bucket(id)
			if ctrExists == nil {
				return fmt.Errorf("state is inconsistent - container ID %s in all containers, but container not found", string(id))
			}

			rawContainerConfig, err := getContainerConfigFromDB(id, ctrBucket)
			if err != nil {
				return fmt.Errorf("error retrieving container %s from the database: %v", string(id), err)
			}

			var containerConfig ContainerConfig
			if err := json.Unmarshal(rawContainerConfig, &containerConfig); err != nil {
				return fmt.Errorf("error unmarshalling container config: %s", err)
			}

			rawContainerState, err := getContainerStateFromDB(id, ctrBucket)
			if err != nil {
				return fmt.Errorf("error retrieving container %s from the database: %v", string(id), err)
			}

			var containerState ContainerState
			if err := json.Unmarshal(rawContainerState, &containerState); err != nil {
				return fmt.Errorf("error unmarshalling container config: %s", err)
			}

			res = append(res, Container{
				Config: &containerConfig,
				State:  &containerState,
			})

			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	return res, nil
}

// Note: original function comes from https://github.com/containers/podman/blob/v3.4.1/libpod/boltdb_state_internal.go
// It was adapted as we don't need to write any information to the DB.
func (client *DBClient) getDBCon() (*bolt.DB, error) {
	dbOptions := bolt.DefaultOptions
	dbOptions.ReadOnly = true
	dbOptions.Timeout = openTimeout

	// Using a custom `OpenFile` to remove the O_CREATE option as we never want to create a file
	dbOptions.OpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		return os.OpenFile(name, flag&^os.O_CREATE, perm)
	}

	db, err := bolt.Open(client.DBPath, 0o0, dbOptions)
	if err != nil {
		return nil, fmt.Errorf("error opening database %s, err: %w", client.DBPath, err)
	}

	return db, nil
}

func getCtrBucket(tx *bolt.Tx) (*bolt.Bucket, error) {
	bkt := tx.Bucket(ctrBkt)
	if bkt == nil {
		return nil, errors.New("containers bucket not found in DB")
	}
	return bkt, nil
}

func getAllCtrsBucket(tx *bolt.Tx) (*bolt.Bucket, error) {
	bkt := tx.Bucket(allCtrsBkt)
	if bkt == nil {
		return nil, errors.New("all containers bucket not found in DB")
	}
	return bkt, nil
}

func getContainerConfigFromDB(id []byte, ctrsBkt *bolt.Bucket) ([]byte, error) {
	ctrBkt := ctrsBkt.Bucket(id)
	if ctrBkt == nil {
		return nil, fmt.Errorf("container %s not found in DB", string(id))
	}

	configBytes := ctrBkt.Get(configKey)
	if configBytes == nil {
		return nil, fmt.Errorf("container %s missing config key in DB", string(id))
	}

	return configBytes, nil
}

func getContainerStateFromDB(id []byte, ctrsBkt *bolt.Bucket) ([]byte, error) {
	ctrBkt := ctrsBkt.Bucket(id)
	if ctrBkt == nil {
		return nil, fmt.Errorf("container %s not found in DB", string(id))
	}

	stateBytes := ctrBkt.Get(stateKey)
	if stateBytes == nil {
		return nil, fmt.Errorf("container %s missing state key in DB", string(id))
	}

	return stateBytes, nil
}
