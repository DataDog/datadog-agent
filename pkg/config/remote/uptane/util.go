// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package uptane

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	bbolterr "go.etcd.io/bbolt/errors"
)

type metaPath struct {
	role       role
	version    uint64
	versionSet bool
}

func parseMetaPath(rawMetaPath string) (metaPath, error) {
	splitRawMetaPath := strings.SplitN(rawMetaPath, ".", 3)
	if len(splitRawMetaPath) != 2 && len(splitRawMetaPath) != 3 {
		return metaPath{}, fmt.Errorf("invalid metadata path '%s'", rawMetaPath)
	}
	suffix := splitRawMetaPath[len(splitRawMetaPath)-1]
	if suffix != "json" {
		return metaPath{}, fmt.Errorf("invalid metadata path (suffix) '%s'", rawMetaPath)
	}
	rawRole := splitRawMetaPath[len(splitRawMetaPath)-2]
	if rawRole == "" {
		return metaPath{}, fmt.Errorf("invalid metadata path (role) '%s'", rawMetaPath)
	}
	if len(splitRawMetaPath) == 2 {
		return metaPath{
			role: role(rawRole),
		}, nil
	}
	rawVersion, err := strconv.ParseUint(splitRawMetaPath[0], 10, 64)
	if err != nil {
		return metaPath{}, fmt.Errorf("invalid metadata path (version) '%s': %w", rawMetaPath, err)
	}
	return metaPath{
		role:       role(rawRole),
		version:    rawVersion,
		versionSet: true,
	}, nil
}

func unsafeMetaVersion(rawMeta json.RawMessage) (uint64, error) {
	var metaVersion struct {
		Signed *struct {
			Version *uint64 `json:"version"`
		} `json:"signed"`
	}
	err := json.Unmarshal(rawMeta, &metaVersion)
	if err != nil {
		return 0, err
	}
	if metaVersion.Signed == nil || metaVersion.Signed.Version == nil {
		return 0, errors.New("invalid meta: version field is missing")
	}
	return *metaVersion.Signed.Version, nil
}

func unsafeMetaCustom(rawMeta json.RawMessage) ([]byte, error) {
	var metaVersion struct {
		Signed *struct {
			Custom json.RawMessage `json:"custom"`
		} `json:"signed"`
	}
	err := json.Unmarshal(rawMeta, &metaVersion)
	if err != nil {
		return nil, err
	}
	if metaVersion.Signed == nil {
		return nil, errors.New("invalid meta: signed is missing")
	}
	return []byte(metaVersion.Signed.Custom), nil
}

func unsafeMetaExpires(rawMeta json.RawMessage) (time.Time, error) {
	var metaExpires struct {
		Signed *struct {
			Expires time.Time `json:"expires"`
		} `json:"signed"`
	}
	err := json.Unmarshal(rawMeta, &metaExpires)
	if err != nil {
		return time.Time{}, err
	}
	if metaExpires.Signed == nil {
		return time.Time{}, errors.New("invalid meta: signed is missing")
	}
	return metaExpires.Signed.Expires, nil
}

func metaHash(rawMeta json.RawMessage) string {
	hash := sha256.Sum256(rawMeta)
	return hex.EncodeToString(hash[:])
}

func trimHashTargetPath(targetPath string) string {
	basename := path.Base(targetPath)
	split := strings.SplitN(basename, ".", 2)
	if len(split) > 1 {
		basename = split[1]
	}
	return path.Join(path.Dir(targetPath), basename)
}

type bufferDestination struct {
	bytes.Buffer
}

func (b *bufferDestination) Delete() error {
	return nil
}

type snapshotCustomData struct {
	OrgUUID *string `json:"org_uuid"`
}

func snapshotCustom(rawCustom []byte) (*snapshotCustomData, error) {
	var custom snapshotCustomData
	if len(rawCustom) == 0 {
		return &custom, nil
	}
	err := json.Unmarshal(rawCustom, &custom)
	if err != nil {
		return nil, err
	}
	return &custom, nil
}

const metaBucket = "meta"
const metaFile = "meta.json"
const databaseLockTimeout = time.Second

// AgentMetadata is data stored in bolt DB to determine whether or not
// the agent has changed and the RC cache should be cleared
type AgentMetadata struct {
	Version      string    `json:"version"`
	APIKeyHash   string    `json:"api-key-hash"`
	CreationTime time.Time `json:"creation-time"`
	URL          string    `json:"url"`
}

// hashAPIKey hashes the API key to avoid storing it in plain text using SHA256
func hashAPIKey(apiKey string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(apiKey)))
}

func recreate(path string, agentVersion string, apiKeyHash string, url string) (*bbolt.DB, error) {
	log.Infof("Clear remote configuration database")
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("could not check if rc db exists: (%s): %v", path, err)
	}
	if err == nil {
		err := os.Remove(path)
		if err != nil {
			return nil, fmt.Errorf("could not remove existing rc db (%s): %v", path, err)
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
		if errors.Is(err, bbolterr.ErrTimeout) {
			return nil, errors.New("rc db is locked. Please check if another instance of the agent is running and using the same `run_path` parameter")
		}
		return nil, err
	}
	return db, addMetadata(db, agentVersion, apiKeyHash, url)
}

func addMetadata(db *bbolt.DB, agentVersion string, apiKeyHash string, url string) error {
	return db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(metaBucket))
		if err != nil {
			return err
		}
		metaData, err := json.Marshal(AgentMetadata{
			Version:      agentVersion,
			APIKeyHash:   apiKeyHash,
			CreationTime: time.Now(),
			URL:          url,
		})
		if err != nil {
			return err
		}
		return bucket.Put([]byte(metaFile), metaData)
	})
}

func getMetadata(db *bbolt.DB) (AgentMetadata, error) {
	var metadata AgentMetadata
	var err error
	err = db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(metaBucket))
		if bucket == nil {
			log.Infof("Missing meta bucket")
			return errors.New("could not get RC metadata: missing bucket")
		}
		metadataBytes := bucket.Get([]byte(metaFile))
		if metadataBytes == nil {
			log.Infof("Missing meta file in meta bucket")
			return errors.New("could not get RC metadata: missing meta file")
		}
		err = json.Unmarshal(metadataBytes, &metadata)
		if err != nil {
			log.Infof("Invalid metadata")
			return err
		}
		return nil
	})
	return metadata, err
}

func openCacheDB(path string, agentVersion string, apiKey string, url string) (*bbolt.DB, error) {
	apiKeyHash := hashAPIKey(apiKey)

	db, err := bbolt.Open(path, 0600, &bbolt.Options{
		Timeout: databaseLockTimeout,
	})
	if err != nil {
		if errors.Is(err, bbolterr.ErrTimeout) {
			return nil, errors.New("rc db is locked. Please check if another instance of the agent is running and using the same `run_path` parameter")
		}
		log.Infof("Failed to open remote configuration database %s", err)
		return recreate(path, agentVersion, apiKeyHash, url)
	}

	metadata, err := getMetadata(db)
	if err != nil {
		_ = db.Close()
		log.Infof("Failed to validate remote configuration database %s", err)
		return recreate(path, agentVersion, apiKeyHash, url)
	}

	if metadata.Version != agentVersion || metadata.APIKeyHash != apiKeyHash || metadata.URL != url {
		log.Infof("Different agent version, API Key or URL detected")
		_ = db.Close()
		return recreate(path, agentVersion, apiKeyHash, url)
	}

	return db, nil
}
