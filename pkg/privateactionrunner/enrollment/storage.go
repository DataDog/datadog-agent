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
	"os"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// IdentityStorage loads and persists the runner identity.
type IdentityStorage interface {
	// Load returns the stored identity, or nil if there is none.
	Load(ctx context.Context) (*PersistedIdentity, error)
	Persist(ctx context.Context, result *Result) error
}

// LeaderSource reports whether this replica is the leader and notifies leadership changes.
type LeaderSource interface {
	IsLeader() bool
	Subscribe() (leadershipChange <-chan struct{}, isLeader func() bool)
}

// NewIdentityStorage returns the K8s secret storage on the Cluster Agent when configured, the identity file otherwise.
// leader is only used by the K8s secret storage.
func NewIdentityStorage(source model.Reader, leader LeaderSource) IdentityStorage {
	cfg := parconfig.New(source)
	if cfg.IdentityUseK8sSecret() && flavor.GetFlavor() == flavor.ClusterAgent {
		return newK8sIdentityStorage(cfg, leader)
	}
	return &fileIdentityStorage{cfg: cfg}
}

type fileIdentityStorage struct {
	cfg parconfig.Config
}

func (s *fileIdentityStorage) Load(_ context.Context) (*PersistedIdentity, error) {
	return getIdentityFromFile(s.cfg)
}

func (s *fileIdentityStorage) Persist(_ context.Context, result *Result) error {
	return persistIdentityToFile(s.cfg, result)
}

// RotateIdentity persists a new identity for explicit rotation.
func RotateIdentity(ctx context.Context, cfg parconfig.Config, result *Result) error {
	if cfg.IdentityUseK8sSecret() && flavor.GetFlavor() == flavor.ClusterAgent {
		return persistIdentityToK8sSecretNoLeader(ctx, cfg, result)
	}
	return persistIdentityToFile(cfg, result)
}

// getIdentityFromFile returns the identity of the private action runner from the identity file. Returns nil if the identity file does not exist
func getIdentityFromFile(cfg parconfig.Config) (*PersistedIdentity, error) {
	filePath := cfg.IdentityFilePath()
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
func persistIdentityToFile(cfg parconfig.Config, result *Result) error {
	filePath := cfg.IdentityFilePath()

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
