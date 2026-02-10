// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package enrollment

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/regions"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

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
func SelfEnroll(ctx context.Context, ddSite, runnerName, apiKey, appKey string) (*Result, error) {
	privateJwk, publicJwk, err := util.GenerateKeys()
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	ddBaseURL := "https://api." + ddSite
	publicClient := opms.NewPublicClient(ddBaseURL)

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

// ConvertResultToIdentityData converts an enrollment result to the data needed for identity storage
func ConvertResultToIdentityData(result *Result) (privateKey string, urn string, err error) {
	privateKeyJWK, err := util.EcdsaToJWK(result.PrivateKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to convert private key to JWK: %w", err)
	}

	marshalledPrivateKey, err := privateKeyJWK.MarshalJSON()
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal private key to JSON: %w", err)
	}

	encodedPrivateKey := base64.RawURLEncoding.EncodeToString(marshalledPrivateKey)
	return encodedPrivateKey, result.URN, nil
}
