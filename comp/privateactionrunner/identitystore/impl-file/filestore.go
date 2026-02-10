// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package filestoreimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	identitystore "github.com/DataDog/datadog-agent/comp/privateactionrunner/identitystore/def"
)

// Configuration keys for the private action runner.
// These mirror the constants in pkg/config/setup but are defined here
// because comp/ packages cannot import pkg/config/setup (depguard rule).
const (
	parIdentityUseK8sSecret = "private_action_runner.use_k8s_secret"
	parIdentityFilePath     = "private_action_runner.identity_file_path"
	defaultIdentityFileName = "privateactionrunner_private_identity.json"
)

// Requires defines the dependencies for the file-based identity store
type Requires struct {
	compdef.In

	Config config.Component
	Log    log.Component
}

// Provides defines the output of the file-based identity store
type Provides struct {
	compdef.Out

	Comp identitystore.Component
}

type fileStore struct {
	config config.Component
	log    log.Component
}

// NewComponent creates a new file-based identity store
func NewComponent(reqs Requires) Provides {
	return Provides{
		Comp: &fileStore{
			config: reqs.Config,
			log:    reqs.Log,
		},
	}
}

func (f *fileStore) GetIdentity(ctx context.Context) (*identitystore.Identity, error) {
	filePath := f.getIdentityFilePath()

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		f.log.Debugf("Identity file does not exist at path: %s", filePath)
		return nil, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read identity file: %w", err)
	}

	var identity identitystore.Identity
	if err := json.Unmarshal(data, &identity); err != nil {
		return nil, fmt.Errorf("failed to parse identity file JSON: %w", err)
	}

	if identity.URN == "" {
		return nil, errors.New("URN is empty in identity file")
	}
	if identity.PrivateKey == "" {
		return nil, errors.New("private key is empty in identity file")
	}

	f.log.Infof("Loaded PAR identity from file: %s", filePath)
	return &identity, nil
}

func (f *fileStore) PersistIdentity(ctx context.Context, identity *identitystore.Identity) error {
	filePath := f.getIdentityFilePath()

	jsonData, err := json.Marshal(identity)
	if err != nil {
		return fmt.Errorf("failed to marshal identity to JSON: %w", err)
	}

	if err := os.WriteFile(filePath, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write identity file: %w", err)
	}

	f.log.Infof("Private Runner identity successfully persisted to %s", filePath)
	return nil
}

func (f *fileStore) DeleteIdentity(ctx context.Context) error {
	filePath := f.getIdentityFilePath()

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil // Already deleted
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to delete identity file: %w", err)
	}

	f.log.Infof("Deleted identity file: %s", filePath)
	return nil
}

// getIdentityFilePath returns the path to the identity file
func (f *fileStore) getIdentityFilePath() string {
	// Priority 1: Explicit config
	if configPath := f.config.GetString(parIdentityFilePath); configPath != "" {
		return configPath
	}

	// Priority 2: Next to auth_token_file_path
	// similarly to pkg/api/security/cert/cert_getter.go we also check if auth_token_file_path as a fallback since customers would probably want these files to be next to each other
	if authTokenPath := f.config.GetString("auth_token_file_path"); authTokenPath != "" {
		dest := filepath.Join(filepath.Dir(authTokenPath), defaultIdentityFileName)
		return dest
	}

	// Priority 3: Next to datadog.yaml
	return filepath.Join(filepath.Dir(f.config.ConfigFileUsed()), defaultIdentityFileName)
}
