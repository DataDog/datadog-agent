// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package enrollment

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	configModel "github.com/DataDog/datadog-agent/pkg/config/model"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// IdentityStorage loads and persists the runner identity.
type IdentityStorage interface {
	// Load returns the stored identity, or nil if there is none.
	Load(ctx context.Context) (*PersistedIdentity, error)
	Persist(ctx context.Context, result *Result) error
	// PinAPIKeyHash records apiKeyHash on the stored identity unless it already has one.
	PinAPIKeyHash(ctx context.Context, apiKeyHash string) error
}

// LeaderSource reports whether this replica is the leader and notifies leadership changes.
type LeaderSource interface {
	IsLeader() bool
	Subscribe() (leadershipChange <-chan struct{}, isLeader func() bool)
}

// NewIdentityStorage returns the K8s secret storage on the Cluster Agent when configured, the identity file otherwise.
// leader is only used by the K8s secret storage.
func NewIdentityStorage(cfg configModel.Reader, leader LeaderSource) IdentityStorage {
	if cfg.GetBool(par.IdentityUseK8sSecret) && flavor.GetFlavor() == flavor.ClusterAgent {
		return newK8sIdentityStorage(cfg, leader)
	}
	return &fileIdentityStorage{cfg: cfg}
}

type fileIdentityStorage struct {
	cfg configModel.Reader
}

func (s *fileIdentityStorage) Load(_ context.Context) (*PersistedIdentity, error) {
	return getIdentityFromFile(s.cfg)
}

func (s *fileIdentityStorage) Persist(_ context.Context, result *Result) error {
	return persistIdentityToFile(s.cfg, result)
}

func (s *fileIdentityStorage) PinAPIKeyHash(_ context.Context, apiKeyHash string) error {
	identity, err := getIdentityFromFile(s.cfg)
	if err != nil {
		return fmt.Errorf("failed to record api_key hash: %w", withoutPath(err))
	}
	if identity == nil || identity.APIKeyHash != "" {
		return nil
	}
	identity.APIKeyHash = apiKeyHash
	jsonData, err := json.Marshal(identity)
	if err != nil {
		return fmt.Errorf("failed to record api_key hash: %w", err)
	}
	if err := os.WriteFile(GetIdentityFilePath(s.cfg), jsonData, 0600); err != nil {
		return fmt.Errorf("failed to record api_key hash: %w", withoutPath(err))
	}
	return nil
}

// withoutPath drops the file path that OS errors carry, as PAR logs are shipped to Datadog.
func withoutPath(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}

// RotateIdentity persists a new identity for explicit rotation.
func RotateIdentity(ctx context.Context, cfg configModel.Reader, result *Result) error {
	if cfg.GetBool(par.IdentityUseK8sSecret) && flavor.GetFlavor() == flavor.ClusterAgent {
		return persistIdentityToK8sSecretNoLeader(ctx, cfg, result)
	}
	return persistIdentityToFile(cfg, result)
}

// getIdentityFromFile returns the identity of the private action runner from the identity file. Returns nil if the identity file does not exist
func getIdentityFromFile(cfg configModel.Reader) (*PersistedIdentity, error) {
	filePath := GetIdentityFilePath(cfg)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read identity file: %w", err)
	}

	var identityContent PersistedIdentity
	if err := json.Unmarshal(data, &identityContent); err != nil {
		return nil, fmt.Errorf("failed to parse identity file JSON: %w", err)
	}

	if identityContent.URN == "" {
		return nil, errors.New("URN is empty in identity file")
	}
	if identityContent.PrivateKey == "" {
		return nil, errors.New("private key is empty in identity file")
	}

	return &identityContent, nil
}

// persistIdentityToFile saves the enrollment result to the identity file
func persistIdentityToFile(cfg configModel.Reader, result *Result) error {
	filePath := GetIdentityFilePath(cfg)

	privateKeyJWK, err := util.EcdsaToJWK(result.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to convert private key to JWK: %w", err)
	}
	marshalledPrivateKey, err := privateKeyJWK.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal private key to JSON: %w", err)
	}

	jsonData, err := json.Marshal(PersistedIdentity{
		PrivateKey: base64.RawURLEncoding.EncodeToString(marshalledPrivateKey),
		URN:        result.URN,
		Hostname:   result.Hostname,
		APIKeyHash: result.APIKeyHash,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal identity content to JSON: %w", err)
	}

	if err := os.WriteFile(filePath, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write temporary identity file: %w", err)
	}

	log.Infof("Private Runner identity successfully persisted to %s", filePath)
	return nil
}

// GetIdentityFilePath returns the path used to persist the runner's self-enrollment identity.
func GetIdentityFilePath(cfg configModel.Reader) string {
	if configPath := cfg.GetString(par.IdentityFilePath); configPath != "" {
		return configPath
	}
	// similarly to pkg/api/security/cert/cert_getter.go we also check if auth_token_file_path as a fallback since customers would probably want these files to be next to each other
	if cfg.GetString("auth_token_file_path") != "" {
		return filepath.Join(filepath.Dir(cfg.GetString("auth_token_file_path")), defaultIdentityFileName)
	}
	return filepath.Join(filepath.Dir(cfg.ConfigFileUsed()), defaultIdentityFileName)
}
