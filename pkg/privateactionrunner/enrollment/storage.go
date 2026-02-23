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
	"path/filepath"

	configModel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// GetIdentityFromPreviousEnrollment retrieves PAR identity from either K8s secret or file based on configuration
func GetIdentityFromPreviousEnrollment(ctx context.Context, cfg configModel.Reader) (*PersistedIdentity, error) {
	if cfg.GetBool(setup.PARIdentityUseK8sSecret) && flavor.GetFlavor() == flavor.ClusterAgent {
		return getIdentityFromK8sSecret(ctx, cfg)
	}
	return getIdentityFromFile(cfg)
}

// PersistIdentity persists identity to either K8s secret or file based on configuration
func PersistIdentity(ctx context.Context, cfg configModel.Reader, result *Result) error {
	if cfg.GetBool(setup.PARIdentityUseK8sSecret) && flavor.GetFlavor() == flavor.ClusterAgent {
		return persistIdentityToK8sSecret(ctx, cfg, result)
	}
	return persistIdentityToFile(cfg, result)
}

// getIdentityFromFile returns the identity of the private action runner from the identity file. Returns nil if the identity file does not exist
func getIdentityFromFile(cfg configModel.Reader) (*PersistedIdentity, error) {
	filePath := getIdentityFilePath(cfg)
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
	filePath := getIdentityFilePath(cfg)

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

// getIdentityFilePath returns the path to the file which contains the identity of the private action runner when doing self-enrollment
func getIdentityFilePath(cfg configModel.Reader) string {
	if configPath := cfg.GetString(setup.PARIdentityFilePath); configPath != "" {
		return configPath
	}
	// similarly to pkg/api/security/cert/cert_getter.go we also check if auth_token_file_path as a fallback since customers would probably want these files to be next to each other
	if cfg.GetString("auth_token_file_path") != "" {
		dest := filepath.Join(filepath.Dir(cfg.GetString("auth_token_file_path")), defaultIdentityFileName)
		log.Warnf("IPC cert/key created or retrieved next to auth_token_file_path location: %v", dest)
		return dest
	}
	return filepath.Join(filepath.Dir(cfg.ConfigFileUsed()), defaultIdentityFileName)
}
