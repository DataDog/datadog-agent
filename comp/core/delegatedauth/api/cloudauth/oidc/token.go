// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package oidc

// Ported from datadog-api-client-go auth/oidc so binaries don't link its HTTP client; TestMatchesClientLibrary guards drift.

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	audiencePrefix = "datadog/"
	proofPrefix    = "oidc-"
	maxTokenSize   = 64 * 1024
)

// TokenScopeError reports why a workload token was refused before contacting Datadog.
type TokenScopeError struct {
	Reason string
}

func (err *TokenScopeError) Error() string {
	return "workload token is not scoped to Datadog: " + err.Reason
}

func proof(tokenFile, orgUUID string, now time.Time) (string, error) {
	if orgUUID == "" {
		return "", errors.New("missing org UUID in delegated token configuration")
	}
	token, err := readToken(tokenFile)
	if err != nil {
		return "", err
	}
	if err := checkDatadogScoped(token, orgUUID, now); err != nil {
		return "", err
	}
	return proofPrefix + token, nil
}

func readToken(path string) (string, error) {
	content, err := readTokenFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(content))
	if token == "" {
		return "", errors.New("workload token is empty")
	}
	if len(token) > maxTokenSize {
		return "", fmt.Errorf("workload token exceeds %d bytes", maxTokenSize)
	}
	for i := 0; i < len(token); i++ {
		if token[i] < 0x21 || token[i] > 0x7e {
			return "", errors.New("workload token contains whitespace or non-printable characters")
		}
	}
	return token, nil
}

// readTokenFile refuses non-regular files before and after opening, because opening a FIFO blocks.
func readTokenFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("opening workload token file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("workload token file is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening workload token file: %w", err)
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("workload token file is not a regular file")
	}
	content, err := io.ReadAll(io.LimitReader(file, maxTokenSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading workload token file: %w", err)
	}
	return content, nil
}

type audience []string

func (value *audience) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*value = audience{single}
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return errors.New("aud is neither a string nor a list of strings")
	}
	*value = list
	return nil
}

type scopeClaims struct {
	Audience audience
	Expiry   *float64
	OrgUUID  string
}

// checkDatadogScoped requires a single datadog/{orgUUID} audience; it does not verify the signature.
func checkDatadogScoped(token, orgUUID string, now time.Time) error {
	claims, err := decodeClaims(token)
	if err != nil {
		return &TokenScopeError{Reason: err.Error()}
	}
	want := audiencePrefix + orgUUID
	// Reasons omit claim and org values because they reach logs and support flares.
	switch {
	case len(claims.Audience) == 0:
		return &TokenScopeError{Reason: "no aud claim"}
	case len(claims.Audience) > 1:
		return &TokenScopeError{Reason: fmt.Sprintf("aud has %d values; only a single Datadog audience for the configured org is accepted", len(claims.Audience))}
	case claims.Audience[0] != want && strings.HasPrefix(claims.Audience[0], audiencePrefix):
		return &TokenScopeError{Reason: "aud is a datadog/ audience that does not match the configured org UUID"}
	case claims.Audience[0] != want:
		return &TokenScopeError{Reason: "aud is not a Datadog audience"}
	}
	if claims.OrgUUID != "" && claims.OrgUUID != orgUUID {
		return &TokenScopeError{Reason: "dd_org_uuid does not match the configured org UUID"}
	}
	if claims.Expiry == nil {
		return &TokenScopeError{Reason: "no exp claim"}
	}
	if expiry := time.Unix(int64(*claims.Expiry), 0); !now.Before(expiry) {
		return &TokenScopeError{Reason: "token expired at " + expiry.UTC().Format(time.RFC3339)}
	}
	return nil
}

func decodeClaims(token string) (*scopeClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("JWT payload is not base64url")
	}
	// Claim names are case-sensitive; struct decoding would let "AUD" override "aud".
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, errors.New("JWT payload is not a JSON object")
	}
	var claims scopeClaims
	if value, ok := raw["aud"]; ok {
		if err := json.Unmarshal(value, &claims.Audience); err != nil {
			return nil, err
		}
	}
	if value, ok := raw["exp"]; ok {
		var expiry float64
		if err := json.Unmarshal(value, &expiry); err != nil {
			return nil, errors.New("exp is not a number")
		}
		claims.Expiry = &expiry
	}
	if value, ok := raw["dd_org_uuid"]; ok {
		if err := json.Unmarshal(value, &claims.OrgUUID); err != nil {
			return nil, errors.New("dd_org_uuid is not a string")
		}
	}
	return &claims, nil
}
