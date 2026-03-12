// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"errors"
	"regexp"
	"strings"
)

var (
	apiKeyRegex = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
	appKeyRegex = regexp.MustCompile(`^([a-f0-9]{40}|ddapp_[a-zA-Z0-9]{34})$`)
)

// isEncrypted reports whether s is an unresolved secret placeholder in the
// ENC[...] format used by the Datadog secret backend.
func isEncrypted(s string) bool {
	s = strings.Trim(s, " \t")
	return strings.HasPrefix(s, "ENC[") && strings.HasSuffix(s, "]")
}

// ValidateAppKey checks whether key is a well-formed Datadog application key.
func ValidateAppKey(key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	if isEncrypted(key) {
		return false, errors.New("app_key contains unresolved secret (ENC[...] format). Check secret_backend_command/secret_backend_type configuration")
	}
	return appKeyRegex.MatchString(key), nil
}

// ValidateAPIKey checks whether key is a well-formed Datadog API key
func ValidateAPIKey(key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	if isEncrypted(key) {
		return false, errors.New("api_key contains unresolved secret (ENC[...] format). Check secret_backend_command/secret_backend_type configuration")
	}
	return apiKeyRegex.MatchString(key), nil
}
