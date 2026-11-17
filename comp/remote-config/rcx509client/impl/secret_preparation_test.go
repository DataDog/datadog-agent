// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcx509clientimpl

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	rcsecretsv1 "github.com/DataDog/libdd-rc/ffi-hosts/go/rcproto/magic_tunnel/remote_config/rc_secrets"
)

const (
	testToken   = "secret-workload-token"
	testProduct = "ndm"
)

type fakeKeys struct {
	publicKey []byte
	err       error
	calls     int
}

func (f *fakeKeys) NewKey(_, _ string) ([]byte, time.Duration, error) {
	f.calls++
	return f.publicKey, 30 * time.Second, f.err
}

func prepareRequest(t *testing.T, product, contextID string) []byte {
	t.Helper()
	payload, err := proto.Marshal(&rcsecretsv1.PrepareSecretDeliveryRequest{
		ConnectionId:        "conn-1",
		EncryptionContextId: contextID,
		Product:             product,
	})
	require.NoError(t, err)
	return payload
}

func newTestPreparer(t *testing.T, token workloadTokenSource, keys encryptionKeySource) *secretPreparer {
	t.Helper()
	return newSecretPreparer([]string{testProduct, ""}, token, keys, logmock.New(t))
}

func okToken(context.Context) (string, error) { return testToken, nil }

func decodePrepareResponse(t *testing.T, raw []byte) *rcsecretsv1.PrepareSecretDeliveryResponse {
	t.Helper()
	var response rcsecretsv1.PrepareSecretDeliveryResponse
	require.NoError(t, proto.Unmarshal(raw, &response))
	return &response
}

func TestPrepareSecretDeliverySuccess(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	preparer := newTestPreparer(t, okToken, &fakeKeys{publicKey: key})

	raw, err := preparer.prepare(context.Background(), 7, prepareRequest(t, testProduct, "ctx-1"))
	require.NoError(t, err)

	response := decodePrepareResponse(t, raw)
	require.Equal(t, rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_OK, response.GetStatus())
	require.Equal(t, testToken, response.GetToken())
	require.Equal(t, key, response.GetPublicKey())
	require.Equal(t, rcsecretsv1.KeyType_KEY_TYPE_NACL_SEALED_BOX_X25519, response.GetKeyType())
	require.Equal(t, 30*time.Second, response.GetKeyValidity().AsDuration())
}

func TestPrepareSecretDeliveryStatuses(t *testing.T) {
	tests := map[string]struct {
		product string
		token   workloadTokenSource
		status  rcsecretsv1.PrepareSecretDeliveryStatus
	}{
		"unknown product": {
			product: "other",
			token:   okToken,
			status:  rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_PRODUCT_UNKNOWN,
		},
		"token error": {
			product: testProduct,
			token:   func(context.Context) (string, error) { return "", errors.New("exchange failed") },
			status:  rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_IDENTITY_UNAVAILABLE,
		},
		"empty token": {
			product: testProduct,
			token:   func(context.Context) (string, error) { return "", nil },
			status:  rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_IDENTITY_UNAVAILABLE,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			keys := &fakeKeys{publicKey: []byte("key")}
			raw, err := newTestPreparer(t, tt.token, keys).prepare(context.Background(), 1, prepareRequest(t, tt.product, "ctx-1"))
			require.NoError(t, err)

			response := decodePrepareResponse(t, raw)
			require.Equal(t, tt.status, response.GetStatus())
			require.Empty(t, response.GetToken())
			require.Empty(t, response.GetPublicKey())
			require.Zero(t, keys.calls, "no key may be offered without a successful preparation")
		})
	}
}

func TestPrepareSecretDeliveryUnknownProductSkipsTokenExchange(t *testing.T) {
	called := false
	token := func(context.Context) (string, error) { called = true; return testToken, nil }
	_, err := newTestPreparer(t, token, &fakeKeys{}).prepare(context.Background(), 1, prepareRequest(t, "other", "ctx-1"))
	require.NoError(t, err)
	require.False(t, called)
}

func TestPrepareSecretDeliveryCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	token := func(context.Context) (string, error) { cancel(); return testToken, nil }

	raw, err := newTestPreparer(t, token, &fakeKeys{publicKey: []byte("key")}).prepare(ctx, 1, prepareRequest(t, testProduct, "ctx-1"))
	require.NoError(t, err)
	response := decodePrepareResponse(t, raw)
	require.Equal(t, rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_IDENTITY_UNAVAILABLE, response.GetStatus())
	require.Empty(t, response.GetToken())
}

func TestPrepareSecretDeliveryKeyFailure(t *testing.T) {
	_, err := newTestPreparer(t, okToken, &fakeKeys{err: errors.New("no entropy")}).prepare(context.Background(), 1, prepareRequest(t, testProduct, "ctx-1"))
	require.Error(t, err)
	require.NotContains(t, err.Error(), "entropy")
}

func TestPrepareSecretDeliveryRejectsMalformedRequests(t *testing.T) {
	tests := map[string][]byte{
		"not a proto":      []byte("\xff\xff\xff"),
		"too large":        make([]byte, maxPrepareRequestSize+1),
		"missing context":  nil,
		"context too long": nil,
		"missing product":  nil,
		"product too long": nil,
	}
	tests["missing context"] = prepareRequest(t, testProduct, "")
	tests["context too long"] = prepareRequest(t, testProduct, strings.Repeat("c", maxContextIDLength+1))
	tests["missing product"] = prepareRequest(t, "", "ctx-1")
	tests["product too long"] = prepareRequest(t, strings.Repeat("p", maxProductLength+1), "ctx-1")

	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newTestPreparer(t, okToken, &fakeKeys{}).prepare(context.Background(), 1, payload)
			require.Error(t, err)
		})
	}
}

func TestPrepareSecretDeliveryDoesNotLogSecrets(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	var logged []string
	logger := &capturingLogger{Component: logmock.New(t), lines: &logged}
	preparer := newSecretPreparer([]string{testProduct}, okToken, &fakeKeys{publicKey: key}, logger)

	_, err := preparer.prepare(context.Background(), 1, prepareRequest(t, testProduct, "ctx-secret-id"))
	require.NoError(t, err)
	require.NotEmpty(t, logged)
	for _, line := range logged {
		require.NotContains(t, line, testToken)
		require.NotContains(t, line, string(key))
		require.NotContains(t, line, "ctx-secret-id")
	}
}

type capturingLogger struct {
	log.Component
	lines *[]string
}

func (l *capturingLogger) Infof(format string, args ...interface{}) {
	*l.lines = append(*l.lines, fmt.Sprintf(format, args...))
}

func TestPlaceholderKeysAreUnusable(t *testing.T) {
	first, validity, err := placeholderKeys{}.NewKey(testProduct, "ctx-1")
	require.NoError(t, err)
	require.Len(t, first, 32)
	require.Positive(t, validity)

	// Callers must not be able to alter the shared placeholder.
	first[0] ^= 0xff
	second, _, err := placeholderKeys{}.NewKey(testProduct, "ctx-2")
	require.NoError(t, err)
	require.NotEqual(t, first, second)
}

func TestSecretPreparationIsRegisteredOnlyWhenEnabled(t *testing.T) {
	tests := map[string]struct {
		overrides  map[string]interface{}
		registered bool
	}{
		"disabled by default": {overrides: map[string]interface{}{enabledSetting: true}, registered: false},
		"enabled":             {overrides: map[string]interface{}{enabledSetting: true, secretsEnabledSetting: true}, registered: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var got rcx509client.ClientConfig
			deps, _ := newDependencies(t, tt.overrides, func(config rcx509client.ClientConfig) (rcx509client.Client, error) {
				got = config
				return newFakeClient(), nil
			})
			New(deps)
			require.Equal(t, tt.registered, got.PrepareSecretDelivery != nil)
		})
	}
}

func TestSecretPreparationUsesWorkloadIdentity(t *testing.T) {
	tests := map[string]struct {
		overrides map[string]interface{}
		instance  string
	}{
		"primary API key": {
			overrides: map[string]interface{}{"api_key": "default-key"},
			instance:  "api_key",
		},
		"Remote Config API key override": {
			overrides: map[string]interface{}{"api_key": "default-key", "remote_configuration.api_key": "rc-key"},
			instance:  "remote_configuration.api_key",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var gotInstance string
			var gotRequest common.WorkloadTokenRequest
			delegated := &delegatedauthmock.Mock{GetWorkloadTokenFunc: func(_ context.Context, instance string, request common.WorkloadTokenRequest) (string, error) {
				gotInstance, gotRequest = instance, request
				return testToken, nil
			}}

			overrides := map[string]interface{}{
				enabledSetting:        true,
				secretsEnabledSetting: true,
				secretProductsSetting: []string{testProduct},
			}
			for key, value := range tt.overrides {
				overrides[key] = value
			}
			var got rcx509client.ClientConfig
			deps, _ := newDependencies(t, overrides, func(config rcx509client.ClientConfig) (rcx509client.Client, error) {
				got = config
				return newFakeClient(), nil
			})
			deps.DelegatedAuth = delegated
			New(deps)
			require.NotNil(t, got.PrepareSecretDelivery)

			raw, err := got.PrepareSecretDelivery(context.Background(), 1, prepareRequest(t, testProduct, "ctx-1"))
			require.NoError(t, err)
			require.Equal(t, rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_OK, decodePrepareResponse(t, raw).GetStatus())
			require.Equal(t, tt.instance, gotInstance)
			require.Equal(t, []string{"rc_secrets"}, gotRequest.Grants)
			require.Empty(t, gotRequest.JWKThumbprint, "key binding is not used yet")
		})
	}
}

func TestSecretPreparationWithoutDelegatedAuth(t *testing.T) {
	var got rcx509client.ClientConfig
	deps, _ := newDependencies(t, map[string]interface{}{
		enabledSetting:        true,
		secretsEnabledSetting: true,
		secretProductsSetting: []string{testProduct},
	}, func(config rcx509client.ClientConfig) (rcx509client.Client, error) {
		got = config
		return newFakeClient(), nil
	})
	New(deps)

	raw, err := got.PrepareSecretDelivery(context.Background(), 1, prepareRequest(t, testProduct, "ctx-1"))
	require.NoError(t, err)
	require.Equal(t, rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_IDENTITY_UNAVAILABLE, decodePrepareResponse(t, raw).GetStatus())
}
