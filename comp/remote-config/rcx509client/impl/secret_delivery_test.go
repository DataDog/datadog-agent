// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcx509clientimpl

import (
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	authmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	rcsecretsv1 "github.com/DataDog/libdd-rc/ffi-hosts/go/rcproto/magic_tunnel/remote_config/rc_secrets"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"
	"google.golang.org/protobuf/proto"
)

func prepareTestSecret(t *testing.T, handler func(context.Context, uint64, []byte) ([]byte, error), product, id string) *rcsecretsv1.PrepareSecretDeliveryResponse {
	t.Helper()
	payload, err := proto.Marshal(&rcsecretsv1.PrepareSecretDeliveryRequest{Product: product, EncryptionContextId: id})
	require.NoError(t, err)
	raw, err := handler(context.Background(), 1, payload)
	require.NoError(t, err)
	response := new(rcsecretsv1.PrepareSecretDeliveryResponse)
	require.NoError(t, proto.Unmarshal(raw, response))
	return response
}

func sealTestSecret(t *testing.T, response *rcsecretsv1.PrepareSecretDeliveryResponse) []byte {
	t.Helper()
	require.Equal(t, rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_OK, response.Status)
	require.Equal(t, rcsecretsv1.KeyType_KEY_TYPE_NACL_SEALED_BOX_X25519, response.KeyType)
	require.Len(t, response.PublicKey, 32)
	ciphertext, err := box.SealAnonymous(nil, []byte("example secret"), (*[32]byte)(response.PublicKey), rand.Reader)
	require.NoError(t, err)
	return ciphertext
}

func TestSecretDeliveryExchangeFailure(t *testing.T) {
	calls := 0
	s := newSecretDelivery([]string{"example"}, func(context.Context) (string, error) { calls++; return "", errors.New("sensitive provider response") })
	denied := prepareTestSecret(t, s.prepare, "other", "context")
	require.Equal(t, rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_PRODUCT_UNKNOWN, denied.Status)
	require.Zero(t, calls)
	failed := prepareTestSecret(t, s.prepare, "example", "context")
	require.Equal(t, rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_IDENTITY_UNAVAILABLE, failed.Status)
	require.Empty(t, failed.Token)
	require.Empty(t, failed.PublicKey)
	require.Empty(t, s.keys)
	require.NotContains(t, failed.String(), "sensitive")
}

func TestSecretDeliveryKeyLifecycle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSecretDelivery([]string{"example"}, func(context.Context) (string, error) { return "workload-token", nil })
		first := prepareTestSecret(t, s.prepare, "example", "context")
		ciphertext := sealTestSecret(t, first)
		require.Equal(t, "workload-token", first.Token)
		require.Equal(t, secretKeyLifetime, first.KeyValidity.AsDuration())
		time.Sleep(time.Second)
		retry := prepareTestSecret(t, s.prepare, "example", "context")
		require.Equal(t, first.PublicKey, retry.PublicKey)
		require.Equal(t, secretKeyLifetime-time.Second, retry.KeyValidity.AsDuration())
		_, err := s.open("other", "context", ciphertext)
		require.Error(t, err)
		key := s.keys[secretContext{"example", "context"}]
		plaintext, err := s.open("example", "context", ciphertext)
		require.NoError(t, err)
		require.Equal(t, "example secret", string(plaintext))
		require.Equal(t, [32]byte{}, *key.private)
		_, err = s.open("example", "context", ciphertext)
		require.Error(t, err)

		expired := prepareTestSecret(t, s.prepare, "example", "expired")
		time.Sleep(secretKeyLifetime)
		_, err = s.open("example", "expired", sealTestSecret(t, expired))
		require.Error(t, err)
		require.Empty(t, s.keys)

		pending := prepareTestSecret(t, s.prepare, "example", "pending")
		key = s.keys[secretContext{"example", "pending"}]
		s.close()
		require.Equal(t, [32]byte{}, *key.private)
		_, err = s.open("example", "pending", sealTestSecret(t, pending))
		require.Error(t, err)
		closed := prepareTestSecret(t, s.prepare, "example", "closed")
		require.Equal(t, rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_IDENTITY_UNAVAILABLE, closed.Status)
		require.Empty(t, s.keys)
	})
}

func TestSecretDeliveryLimitsAndBadCiphertext(t *testing.T) {
	s := newSecretDelivery([]string{"example"}, func(context.Context) (string, error) { return "workload-token", nil })
	s.capacity = 1
	prepareTestSecret(t, s.prepare, "example", "context")
	request, err := proto.Marshal(&rcsecretsv1.PrepareSecretDeliveryRequest{Product: "example", EncryptionContextId: "another"})
	require.NoError(t, err)
	_, err = s.prepare(context.Background(), 1, request)
	require.ErrorContains(t, err, "too many pending")
	_, err = s.open("example", "context", []byte("invalid ciphertext"))
	require.ErrorContains(t, err, "authentication failed")
	require.Empty(t, s.keys)
	for _, payload := range [][]byte{nil, []byte("invalid protobuf"), make([]byte, 8193)} {
		_, err = s.prepare(context.Background(), 1, payload)
		require.EqualError(t, err, "invalid secret preparation request")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw, err := s.prepare(ctx, 1, request)
	require.NoError(t, err)
	var response rcsecretsv1.PrepareSecretDeliveryResponse
	require.NoError(t, proto.Unmarshal(raw, &response))
	require.Equal(t, rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_IDENTITY_UNAVAILABLE, response.Status)
	require.Empty(t, s.keys)
}

func TestSecretDeliveryConcurrentConsumption(t *testing.T) {
	s := newSecretDelivery([]string{"example"}, func(context.Context) (string, error) { return "workload-token", nil })
	response := prepareTestSecret(t, s.prepare, "example", "context")
	ciphertext := sealTestSecret(t, response)
	var wg sync.WaitGroup
	successes := make(chan bool, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.open("example", "context", ciphertext); successes <- err == nil }()
	}
	wg.Wait()
	close(successes)
	count := 0
	for ok := range successes {
		if ok {
			count++
		}
	}
	require.Equal(t, 1, count)
}

func TestSecretPreparationComponentUsesConfiguredIdentity(t *testing.T) {
	for _, instance := range []string{"api_key", "remote_configuration.api_key"} {
		t.Run(instance, func(t *testing.T) {
			var config rcx509client.ClientConfig
			overrides := map[string]interface{}{enabledSetting: true, secretsEnabledSetting: true, secretProductsSetting: []string{"example"}}
			if instance != "api_key" {
				overrides[instance] = "override-key"
			}
			deps, lifecycle := newDependencies(t, overrides, func(c rcx509client.ClientConfig) (rcx509client.Client, error) {
				config = c
				return newFakeClient(), nil
			})
			calls := 0
			deps.DelegatedAuth = &authmock.Mock{GetWorkloadTokenFunc: func(ctx context.Context, key string, request common.WorkloadTokenRequest) (string, error) {
				require.NoError(t, ctx.Err())
				require.Equal(t, instance, key)
				require.Equal(t, common.WorkloadTokenRequest{Grants: []string{"rc_secrets"}}, request)
				calls++
				return "verified-by-ets", nil
			}}
			comp := New(deps).Comp
			require.NotNil(t, config.PrepareSecretDelivery)
			require.NoError(t, lifecycle.Start(context.Background()))
			response := prepareTestSecret(t, config.PrepareSecretDelivery, "example", "context")
			require.Equal(t, 1, calls)
			require.Equal(t, "verified-by-ets", response.Token)
			plain, err := comp.OpenSecret("example", "context", sealTestSecret(t, response))
			require.NoError(t, err)
			require.Equal(t, "example secret", string(plain))
			pending := prepareTestSecret(t, config.PrepareSecretDelivery, "example", "pending")
			require.NoError(t, lifecycle.Stop(context.Background()))
			_, err = comp.OpenSecret("example", "pending", sealTestSecret(t, pending))
			require.Error(t, err)
		})
	}
}

func TestSecretPreparationDisabledAndMissingIdentity(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		var config rcx509client.ClientConfig
		deps, _ := newDependencies(t, map[string]interface{}{enabledSetting: true, secretsEnabledSetting: enabled, secretProductsSetting: []string{"example"}}, func(c rcx509client.ClientConfig) (rcx509client.Client, error) {
			config = c
			return newFakeClient(), nil
		})
		comp := New(deps).Comp
		if enabled {
			response := prepareTestSecret(t, config.PrepareSecretDelivery, "example", "context")
			require.Equal(t, rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_IDENTITY_UNAVAILABLE, response.Status)
		} else {
			require.Nil(t, config.PrepareSecretDelivery)
		}
		_, err := comp.OpenSecret("example", "context", nil)
		require.Error(t, err)
	}
}

func TestSecretPreparationCannotAllocateAfterShutdown(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s := newSecretDelivery([]string{"example"}, func(context.Context) (string, error) { close(entered); <-release; return "workload-token", nil })
	request, err := proto.Marshal(&rcsecretsv1.PrepareSecretDeliveryRequest{Product: "example", EncryptionContextId: "context"})
	require.NoError(t, err)
	type result struct {
		raw []byte
		err error
	}
	done := make(chan result, 1)
	go func() { raw, err := s.prepare(context.Background(), 1, request); done <- result{raw, err} }()
	<-entered
	s.close()
	close(release)
	got := <-done
	require.NoError(t, got.err)
	var response rcsecretsv1.PrepareSecretDeliveryResponse
	require.NoError(t, proto.Unmarshal(got.raw, &response))
	require.Equal(t, rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_IDENTITY_UNAVAILABLE, response.Status)
	require.Empty(t, response.PublicKey)
	require.Empty(t, s.keys)
}
