// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package enrollment

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	parenrollment "github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	parutil "github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

func splitConfig(t *testing.T, overrides map[string]interface{}) coreconfig.Component {
	values := map[string]interface{}{
		"private_action_runner.enabled":            true,
		"private_action_runner.split_enabled":      true,
		"private_action_runner.identity_file_path": filepath.Join(t.TempDir(), "identity.json"),
	}
	for key, value := range overrides {
		values[key] = value
	}
	return coreconfig.NewMockWithOverrides(t, values)
}

func runEnsure(t *testing.T, cfg coreconfig.Component, request Request, enroll enrollAndPersistFunc) (*Response, error) {
	t.Helper()
	hostnameComp, _ := hostnamemock.NewMock("test-host")
	return ensure(context.Background(), cfg, hostnameComp, request, enroll)
}

func TestPersistedIdentityWins(t *testing.T) {
	cfg := splitConfig(t, nil)
	writeIdentity(t, cfg, validURN(), validPrivateKey(t), "test-host")
	request := Request{ConfiguredIdentity: &ConfiguredIdentity{
		URN: parutil.MakeRunnerURN("us5", 999, "configured-runner"),
	}}

	response, err := runEnsure(t, cfg, request, failIfEnrolled(t))

	require.NoError(t, err)
	assert.Equal(t, validURN(), response.URN)
	assert.Equal(t, int64(123), response.OrgID)
}

func TestUsesRequestIdentity(t *testing.T) {
	urn := parutil.MakeRunnerURN("us5", 999, "configured-runner")
	key := validPrivateKey(t)
	response, err := runEnsure(t, splitConfig(t, nil), Request{ConfiguredIdentity: &ConfiguredIdentity{
		URN:        urn,
		PrivateKey: key,
	}}, failIfEnrolled(t))

	require.NoError(t, err)
	assert.Equal(t, urn, response.URN)
	assert.Equal(t, key, response.PrivateKey)
	assert.NotEmpty(t, response.AgentVersion)
}

func TestRequestIdentityReplacesStalePersistedIdentity(t *testing.T) {
	cfg := splitConfig(t, nil)
	writeIdentity(t, cfg, validURN(), validPrivateKey(t), "old-host")
	configured := &ConfiguredIdentity{
		URN:        parutil.MakeRunnerURN("us5", 999, "configured-runner"),
		PrivateKey: validPrivateKey(t),
	}

	response, err := runEnsure(t, cfg, Request{ConfiguredIdentity: configured}, failIfEnrolled(t))

	require.NoError(t, err)
	assert.Equal(t, configured.URN, response.URN)
	assert.Equal(t, configured.PrivateKey, response.PrivateKey)
}

func TestSelfEnrolls(t *testing.T) {
	cfg := splitConfig(t, map[string]interface{}{"private_action_runner.self_enroll": true})
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	response, err := runEnsure(t, cfg, Request{}, func(ctx context.Context, cfg coreconfig.Component, _ *parenrollment.AgentIdentifier) (*parenrollment.Result, error) {
		result := &parenrollment.Result{URN: validURN(), PrivateKey: privateKey, Hostname: "test-host"}
		return result, parenrollment.PersistIdentity(ctx, cfg, result)
	})

	require.NoError(t, err)
	assert.Equal(t, validURN(), response.URN)
	assert.NotEmpty(t, response.PrivateKey)
}

func TestRejectsDisabledSplitMode(t *testing.T) {
	_, err := runEnsure(t, coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.enabled":       true,
		"private_action_runner.split_enabled": false,
	}), Request{}, failIfEnrolled(t))

	require.ErrorContains(t, err, "split mode is disabled")
}

func TestRejectsPartialRequestIdentity(t *testing.T) {
	_, err := runEnsure(t, splitConfig(t, nil), Request{ConfiguredIdentity: &ConfiguredIdentity{
		URN: validURN(),
	}}, failIfEnrolled(t))

	require.ErrorContains(t, err, "requires both")
}

func TestRejectsInvalidRequestIdentityWithoutLeakingKey(t *testing.T) {
	const secret = "not-a-private-key"
	_, err := runEnsure(t, splitConfig(t, nil), Request{ConfiguredIdentity: &ConfiguredIdentity{
		URN:        validURN(),
		PrivateKey: secret,
	}}, failIfEnrolled(t))

	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)
}

func TestRejectsFIPS(t *testing.T) {
	_, err := runEnsure(t, splitConfig(t, map[string]interface{}{
		"private_action_runner.urn":         validURN(),
		"private_action_runner.private_key": validPrivateKey(t),
		"fips.enabled":                      true,
	}), Request{}, failIfEnrolled(t))

	require.ErrorContains(t, err, "fips.enabled")
}

func writeIdentity(t *testing.T, cfg coreconfig.Component, urn, key, hostname string) {
	t.Helper()
	result := &parenrollment.Result{URN: urn, Hostname: hostname}
	jwk, err := parutil.Base64ToJWK(key)
	require.NoError(t, err)
	result.PrivateKey = jwk.Key.(*ecdsa.PrivateKey)
	require.NoError(t, parenrollment.PersistIdentity(context.Background(), cfg, result))
}

func validURN() string {
	return parutil.MakeRunnerURN("us1", 123, "test-runner")
}

func validPrivateKey(t *testing.T) string {
	t.Helper()
	privateJWK, _, err := parutil.GenerateKeys()
	require.NoError(t, err)
	encoded, err := privateJWK.MarshalJSON()
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func failIfEnrolled(t *testing.T) enrollAndPersistFunc {
	return func(context.Context, coreconfig.Component, *parenrollment.AgentIdentifier) (*parenrollment.Result, error) {
		t.Fatal("unexpected enrollment")
		return nil, nil
	}
}
