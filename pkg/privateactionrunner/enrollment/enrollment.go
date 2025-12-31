// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package enrollment

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	configModel "github.com/DataDog/datadog-agent/pkg/config/model"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/regions"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

const defaultIdentityFileName = "privateactionrunner_private_identity.json"

// Result contains the result of a successful enrollment
type Result struct {
	PrivateKey *ecdsa.PrivateKey
	URN        string
}

type PersistedIdentity struct {
	PrivateKey string `json:"private_key"`
	URN        string `json:"urn"`
}

// SelfEnroll performs self-registration of a private action runner using API credentials
func SelfEnroll(ddSite, runnerName, apiKey, appKey string) (*Result, error) {
	privateJwk, publicJwk, err := util.GenerateKeys()
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	ddBaseURL := "https://api." + ddSite
	publicClient := opms.NewPublicClient(ddBaseURL)

	ctx := context.Background()
	runnerModes := []modes.Mode{modes.ModePull}

	createRunnerResponse, err := publicClient.EnrollWithApiKey(
		ctx,
		apiKey,
		appKey,
		runnerName,
		runnerModes,
		publicJwk,
	)
	if err != nil {
		return nil, fmt.Errorf("enrollment API call failed: %w", err)
	}

	region := regions.GetRegionFromDDSite(ddSite)
	urn := util.MakeRunnerURN(region, createRunnerResponse.OrgID, createRunnerResponse.RunnerID)

	return &Result{
		PrivateKey: privateJwk.Key.(*ecdsa.PrivateKey),
		URN:        urn,
	}, nil
}

// GetIdentityFromPreviousEnrollment returns the identity of the private action runner from the identity file. Returns nil if the identity file does not exist
func GetIdentityFromPreviousEnrollment(cfg configModel.Reader) (*PersistedIdentity, error) {
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

// getIdentityFilePath returns the path to the file which contains the identity of the private action runner when doing self-enrollment
func getIdentityFilePath(cfg configModel.Reader) string {
	if configPath := cfg.GetString("privateactionrunner.identity_file_path"); configPath != "" {
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

// PersistIdentity saves the enrollment result to the identity file
func PersistIdentity(cfg configModel.Reader, result *Result) error {
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
