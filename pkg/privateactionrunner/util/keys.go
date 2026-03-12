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

// AppKeyURLRegex matches a valid Datadog application key embedded as a query
// parameter named "application_key" in a URL. It can be used to detect or
// scrub app keys from log lines and HTTP requests.
//
// Accepted key formats:
//   - [pub]<34 hex chars>def789   (legacy hex key, optional "pub" prefix)
//   - ddapp_<28 alphanumeric chars>def789  (new-style key)
var AppKeyURLRegex = regexp.MustCompile(`.*\?(.*&)?(?i:application_key)=(?:(?i:pub)?[0-9A-Fa-f]{34}|ddapp_[0-9A-Za-z]{28})(?i:def789)(&.*)?`)

// appKeyRegex validates a standalone application key value.
// It accepts the same two formats as AppKeyURLRegex.
var appKeyRegex = regexp.MustCompile(`^(?:(?i:pub)?[0-9A-Fa-f]{34}|ddapp_[0-9A-Za-z]{28})(?i:def789)$`)

// apiKeyRegex matches valid Datadog API keys (32 hexadecimal characters).
var apiKeyRegex = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)

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
