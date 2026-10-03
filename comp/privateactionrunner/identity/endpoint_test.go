// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
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
	return resolve(context.Background(), cfg, hostnameComp, request, enroll)
}

func TestPersistedIdentityWins(t *testing.T) {
	cfg := splitConfig(t, nil)
	writeIdentity(t, cfg, validURN(), validPrivateKey(t), "test-host")
	request := Request{HasLocalIdentity: true}

	response, err := runEnsure(t, cfg, request, failIfEnrolled(t))

	require.NoError(t, err)
	assert.Equal(t, "provided", response.Source)
	assert.Equal(t, validURN(), response.URN)
}

func TestSelectsRequestIdentity(t *testing.T) {
	response, err := runEnsure(t, splitConfig(t, nil), Request{HasLocalIdentity: true}, failIfEnrolled(t))

	require.NoError(t, err)
	assert.Equal(t, &Response{Source: "local"}, response)
}

func TestRequestIdentityReplacesStalePersistedIdentity(t *testing.T) {
	cfg := splitConfig(t, nil)
	writeIdentity(t, cfg, validURN(), validPrivateKey(t), "old-host")
	response, err := runEnsure(t, cfg, Request{HasLocalIdentity: true}, failIfEnrolled(t))

	require.NoError(t, err)
	assert.Equal(t, &Response{Source: "local"}, response)
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
	assert.Equal(t, "provided", response.Source)
	assert.Equal(t, validURN(), response.URN)
	assert.NotEmpty(t, response.PrivateKey)
}

func TestRejectsDisabledSplitMode(t *testing.T) {
	_, err := runEnsure(t, coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.enabled":       true,
		"private_action_runner.split_enabled": false,
	}), Request{}, failIfEnrolled(t))

	require.ErrorContains(t, err, "split mode is disabled")
	assert.Equal(t, http.StatusConflict, statusForError(err))
}

func TestRejectsFIPS(t *testing.T) {
	_, err := runEnsure(t, splitConfig(t, map[string]interface{}{
		"private_action_runner.urn":         validURN(),
		"private_action_runner.private_key": validPrivateKey(t),
		"fips.enabled":                      true,
	}), Request{}, failIfEnrolled(t))

	require.ErrorContains(t, err, "fips.enabled")
	assert.Equal(t, http.StatusConflict, statusForError(err))
}

func TestRejectsMissingIdentityWhenSelfEnrollmentIsDisabled(t *testing.T) {
	cfg := splitConfig(t, map[string]interface{}{"private_action_runner.self_enroll": false})
	_, err := runEnsure(t, cfg, Request{}, failIfEnrolled(t))

	require.ErrorContains(t, err, "self-enrollment is disabled")
	assert.Equal(t, http.StatusConflict, statusForError(err))
}

func TestEnrollmentFailureIsUnavailable(t *testing.T) {
	cfg := splitConfig(t, map[string]interface{}{"private_action_runner.self_enroll": true})
	_, err := runEnsure(t, cfg, Request{}, func(context.Context, coreconfig.Component, *parenrollment.AgentIdentifier) (*parenrollment.Result, error) {
		return nil, errors.New("enrollment unavailable")
	})

	require.ErrorContains(t, err, "enrollment unavailable")
	assert.Equal(t, http.StatusServiceUnavailable, statusForError(err))
}

func TestUnexpectedFailureIsInternalServerError(t *testing.T) {
	assert.Equal(t, http.StatusInternalServerError, statusForError(errors.New("unexpected failure")))
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
