// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"
)

// TestIsActionAllowed_ExactMatch verifies that a specific action listed in the allowlist is
// permitted and an action not in the set is denied.
func TestIsActionAllowed_ExactMatch(t *testing.T) {
	cfg := &Config{
		ActionsAllowlist: map[string]sets.Set[string]{
			"com.datadoghq.kubernetes.core": sets.New[string]("getPods", "listDeployments"),
		},
	}

	assert.True(t, cfg.IsActionAllowed("com.datadoghq.kubernetes.core", "getPods"))
	assert.True(t, cfg.IsActionAllowed("com.datadoghq.kubernetes.core", "listDeployments"))
	assert.False(t, cfg.IsActionAllowed("com.datadoghq.kubernetes.core", "deletePod"))
}

// TestIsActionAllowed_Wildcard verifies that a bundle entry with "*" permits any action name.
func TestIsActionAllowed_Wildcard(t *testing.T) {
	cfg := &Config{
		ActionsAllowlist: map[string]sets.Set[string]{
			"com.datadoghq.http": sets.New[string]("*"),
		},
	}

	assert.True(t, cfg.IsActionAllowed("com.datadoghq.http", "request"))
	assert.True(t, cfg.IsActionAllowed("com.datadoghq.http", "getChecksumFromUrl"))
	assert.True(t, cfg.IsActionAllowed("com.datadoghq.http", "anything"))
}

// TestIsActionAllowed_BundleNotInAllowlist verifies that an action from a bundle absent in the
// allowlist is rejected even if the action name would match another bundle's entry.
func TestIsActionAllowed_BundleNotInAllowlist(t *testing.T) {
	cfg := &Config{
		ActionsAllowlist: map[string]sets.Set[string]{
			"com.datadoghq.http": sets.New[string]("request"),
		},
	}

	assert.False(t, cfg.IsActionAllowed("com.datadoghq.kubernetes.core", "request"))
}

// TestIsActionAllowed_EmptyAllowlist verifies that an empty allowlist denies everything.
func TestIsActionAllowed_EmptyAllowlist(t *testing.T) {
	cfg := &Config{ActionsAllowlist: map[string]sets.Set[string]{}}
	assert.False(t, cfg.IsActionAllowed("com.datadoghq.http", "request"))
}

// TestIsURLInAllowlist_NilAllowlist verifies that a nil allowlist means all URLs are permitted.
// This is the default state before any allowlist is configured.
func TestIsURLInAllowlist_NilAllowlist(t *testing.T) {
	cfg := &Config{Allowlist: nil}
	assert.True(t, cfg.IsURLInAllowlist("https://api.internal.example.com/v1/data"))
}

// TestIsURLInAllowlist_ExactHostnameMatch verifies that an exact hostname in the allowlist
// permits the URL and rejects a different hostname.
func TestIsURLInAllowlist_ExactHostnameMatch(t *testing.T) {
	cfg := &Config{Allowlist: []string{"api.example.com"}}

	assert.True(t, cfg.IsURLInAllowlist("https://api.example.com/v1/path?q=1"))
	assert.False(t, cfg.IsURLInAllowlist("https://other.example.com/path"))
}

// TestIsURLInAllowlist_MatchIsCaseInsensitive verifies that hostname comparison ignores case
// so "API.EXAMPLE.COM" in the allowlist matches "api.example.com" in the URL.
func TestIsURLInAllowlist_MatchIsCaseInsensitive(t *testing.T) {
	cfg := &Config{Allowlist: []string{"API.EXAMPLE.COM"}}
	assert.True(t, cfg.IsURLInAllowlist("https://api.example.com/path"))
}

// TestIsURLInAllowlist_GlobPattern verifies that glob wildcard patterns in the allowlist
// match subdomains while still rejecting unrelated hostnames.
func TestIsURLInAllowlist_GlobPattern(t *testing.T) {
	cfg := &Config{Allowlist: []string{"*.internal.corp"}}

	assert.True(t, cfg.IsURLInAllowlist("https://svc1.internal.corp/api"))
	assert.True(t, cfg.IsURLInAllowlist("https://svc2.internal.corp/"))
	assert.False(t, cfg.IsURLInAllowlist("https://internal.corp/api"))
	assert.False(t, cfg.IsURLInAllowlist("https://external.example.com/api"))
}

// TestIsURLInAllowlist_InvalidURL verifies that an unparseable URL is rejected
// (rather than crashing or incorrectly permitting).
func TestIsURLInAllowlist_InvalidURL(t *testing.T) {
	cfg := &Config{Allowlist: []string{"api.example.com"}}
	assert.False(t, cfg.IsURLInAllowlist("://\x00bad"))
}

// TestIdentityIsIncomplete_MissingURN verifies that a config with a private key but no URN
// is considered incomplete (the runner cannot be identified without both).
func TestIdentityIsIncomplete_MissingURN(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	cfg := &Config{Urn: "", PrivateKey: key}
	assert.True(t, cfg.IdentityIsIncomplete())
}

// TestIdentityIsIncomplete_MissingPrivateKey verifies that a config with a URN but no private
// key is incomplete (the runner cannot sign JWTs without the key).
func TestIdentityIsIncomplete_MissingPrivateKey(t *testing.T) {
	cfg := &Config{Urn: "urn:dd:apps:on-prem-runner:us1:1:r1", PrivateKey: nil}
	assert.True(t, cfg.IdentityIsIncomplete())
}

// TestIdentityIsIncomplete_BothPresent verifies that a fully-configured identity is complete.
func TestIdentityIsIncomplete_BothPresent(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	cfg := &Config{Urn: "urn:dd:apps:on-prem-runner:us1:1:r1", PrivateKey: key}
	assert.False(t, cfg.IdentityIsIncomplete())
}
