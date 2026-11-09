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
	"time"

	rcsecretsv1 "github.com/DataDog/libdd-rc/ffi-hosts/go/rcproto/magic_tunnel/remote_config/rc_secrets"
	"golang.org/x/crypto/nacl/box"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

const prepareSecretDeliveryURI = "rc.x509.magic_tunnel.remote_config.rc_secrets.v1.SecretsService/PrepareSecretDelivery"
const secretKeyLifetime = time.Minute

type secretContext struct{ product, id string }

type secretKey struct {
	public, private *[32]byte
	expires         time.Time
}

type secretDelivery struct {
	mu       sync.Mutex
	keys     map[secretContext]*secretKey
	closed   bool
	products map[string]bool
	token    func(context.Context) (string, error)
	capacity int
}

func newSecretDelivery(products []string, token func(context.Context) (string, error)) *secretDelivery {
	allowed := make(map[string]bool, len(products))
	for _, product := range products {
		if product != "" {
			allowed[product] = true
		}
	}
	return &secretDelivery{keys: make(map[secretContext]*secretKey), products: allowed, token: token, capacity: 128}
}

func secretPreparationStatus(status rcsecretsv1.PrepareSecretDeliveryStatus) ([]byte, error) {
	return proto.Marshal(&rcsecretsv1.PrepareSecretDeliveryResponse{Status: status})
}

func (s *secretDelivery) prepare(ctx context.Context, _ uint64, payload []byte) ([]byte, error) {
	var request rcsecretsv1.PrepareSecretDeliveryRequest
	if len(payload) > 8192 || proto.Unmarshal(payload, &request) != nil || request.EncryptionContextId == "" || len(request.EncryptionContextId) > 256 {
		return nil, errors.New("invalid secret preparation request")
	}
	if !s.products[request.Product] {
		return secretPreparationStatus(rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_PRODUCT_UNKNOWN)
	}
	if ctx.Err() != nil || s.token == nil {
		return secretPreparationStatus(rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_IDENTITY_UNAVAILABLE)
	}
	// The provider exchange happens before allocating a key. Errors and tokens
	// must not enter handler logs or transport error messages.
	token, err := s.token(ctx)
	if err != nil || token == "" || ctx.Err() != nil {
		return secretPreparationStatus(rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_IDENTITY_UNAVAILABLE)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return secretPreparationStatus(rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_IDENTITY_UNAVAILABLE)
	}
	now := time.Now()
	s.expire(now)
	keyID := secretContext{request.Product, request.EncryptionContextId}
	key := s.keys[keyID]
	if key == nil {
		if len(s.keys) >= s.capacity {
			return nil, errors.New("too many pending secret deliveries")
		}
		public, private, err := box.GenerateKey(rand.Reader)
		if err != nil {
			return nil, errors.New("secret encryption key generation failed")
		}
		key = &secretKey{public: public, private: private, expires: now.Add(secretKeyLifetime)}
		s.keys[keyID] = key
	}
	// Repeated preparation of a pending context keeps its key and deadline, so
	// retrying preparation cannot invalidate an already-in-flight delivery.
	return proto.Marshal(&rcsecretsv1.PrepareSecretDeliveryResponse{
		Status:      rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_OK,
		Token:       token,
		PublicKey:   key.public[:],
		KeyType:     rcsecretsv1.KeyType_KEY_TYPE_NACL_SEALED_BOX_X25519,
		KeyValidity: durationpb.New(key.expires.Sub(now)),
	})
}

// open consumes the context even when ciphertext authentication fails. Product
// delivery handlers own decoding and applying the returned plaintext.
func (s *secretDelivery) open(product, contextID string, ciphertext []byte) ([]byte, error) {
	if s == nil {
		return nil, errors.New("secret delivery is disabled")
	}
	s.mu.Lock()
	s.expire(time.Now())
	keyID := secretContext{product, contextID}
	key := s.keys[keyID]
	delete(s.keys, keyID)
	s.mu.Unlock()
	if key == nil {
		return nil, errors.New("secret encryption context is unavailable")
	}
	defer clear(key.private[:])
	plaintext, ok := box.OpenAnonymous(nil, ciphertext, key.public, key.private)
	if !ok {
		return nil, errors.New("secret ciphertext authentication failed")
	}
	return plaintext, nil
}

// expire is called with the mutex held. Storage stays bounded even when a
// backend abandons preparation without delivering a secret.
func (s *secretDelivery) expire(now time.Time) {
	for id, key := range s.keys {
		if !now.Before(key.expires) {
			clear(key.private[:])
			delete(s.keys, id)
		}
	}
}

func (s *secretDelivery) close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for id, key := range s.keys {
		clear(key.private[:])
		delete(s.keys, id)
	}
}

// The demo product only acknowledges this fixed fixture; it never applies secrets
// to Agent configuration. Registration requires the staging site and product opt-in.
const pocProduct = "rc-secrets-poc"
const pocPayload = "rc-secrets-poc: delivered"
const deliverPOCSecretURI = "rc.x509.magic_tunnel.remote_config.rc_secrets.v1.SecretsService/DeliverPOCSecret"

func (s *secretDelivery) deliverPOC(ctx context.Context, _ uint64, payload []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var request rcsecretsv1.DeliverPOCSecretRequest
	if len(payload) > 8192 || proto.Unmarshal(payload, &request) != nil || request.EncryptionContextId == "" || len(request.Ciphertext) > 1024 {
		return nil, errors.New("invalid POC delivery request")
	}
	plaintext, err := s.open(pocProduct, request.EncryptionContextId, request.Ciphertext)
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)
	if string(plaintext) != pocPayload {
		return nil, errors.New("unexpected POC payload")
	}
	return proto.Marshal(&rcsecretsv1.DeliverPOCSecretResponse{Acknowledged: true})
}
