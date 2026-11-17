// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcx509clientimpl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	rcsecretsv1 "github.com/DataDog/libdd-rc/ffi-hosts/go/rcproto/magic_tunnel/remote_config/rc_secrets"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	// Limits match what the backend policy enforces for these fields.
	maxPrepareRequestSize = 8192
	maxContextIDLength    = 128
	maxProductLength      = 64

	placeholderKeyValidity = time.Minute
)

// workloadTokenSource returns a workload identity token granting rc_secrets.
type workloadTokenSource func(ctx context.Context) (string, error)

// encryptionKeySource provides the public key the backend seals secrets to.
type encryptionKeySource interface {
	NewKey(product, contextID string) (publicKey []byte, validity time.Duration, err error)
}

// placeholderPublicKey is derived from a fixed string, so no private key exists
// for it and nothing sealed to it can ever be opened.
var placeholderPublicKey = sha256.Sum256([]byte("rc-secrets placeholder key: no private key exists"))

// placeholderKeys stands in for real key generation until encryption lands.
// TODO: replace with a key source that keeps the private half for decryption.
type placeholderKeys struct{}

func (placeholderKeys) NewKey(_, _ string) ([]byte, time.Duration, error) {
	return bytes.Clone(placeholderPublicKey[:]), placeholderKeyValidity, nil
}

// secretPreparer handles the SecretsService/PrepareSecretDelivery RPC: it proves
// the Agent's identity with a workload token and offers an encryption key.
type secretPreparer struct {
	products map[string]struct{}
	token    workloadTokenSource
	keys     encryptionKeySource
	logger   log.Component
}

func newSecretPreparer(products []string, token workloadTokenSource, keys encryptionKeySource, logger log.Component) *secretPreparer {
	allowed := make(map[string]struct{}, len(products))
	for _, product := range products {
		if product != "" {
			allowed[product] = struct{}{}
		}
	}
	return &secretPreparer{products: allowed, token: token, keys: keys, logger: logger}
}

func (p *secretPreparer) prepare(ctx context.Context, correlationID uint64, payload []byte) ([]byte, error) {
	var request rcsecretsv1.PrepareSecretDeliveryRequest
	if len(payload) > maxPrepareRequestSize || proto.Unmarshal(payload, &request) != nil ||
		request.GetEncryptionContextId() == "" || len(request.GetEncryptionContextId()) > maxContextIDLength ||
		request.GetProduct() == "" || len(request.GetProduct()) > maxProductLength {
		return nil, errors.New("invalid secret preparation request")
	}

	response, err := p.build(ctx, request.GetProduct(), request.GetEncryptionContextId())
	if err != nil {
		return nil, err
	}
	// Never log the token, the key, or request fields other than the product.
	p.logger.Infof("remote config x509 secret preparation handled (correlation_id=%d, product=%s, status=%s)",
		correlationID, request.GetProduct(), response.GetStatus())
	return proto.Marshal(response)
}

func (p *secretPreparer) build(ctx context.Context, product, contextID string) (*rcsecretsv1.PrepareSecretDeliveryResponse, error) {
	if _, ok := p.products[product]; !ok {
		return statusResponse(rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_PRODUCT_UNKNOWN), nil
	}

	token, err := p.token(ctx)
	if err != nil || token == "" || ctx.Err() != nil {
		if err != nil {
			p.logger.Warnf("remote config x509 secret preparation could not get a workload token: %v", err)
		}
		return statusResponse(rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_IDENTITY_UNAVAILABLE), nil
	}

	publicKey, validity, err := p.keys.NewKey(product, contextID)
	if err != nil {
		return nil, errors.New("secret encryption key unavailable")
	}
	return &rcsecretsv1.PrepareSecretDeliveryResponse{
		Status:      rcsecretsv1.PrepareSecretDeliveryStatus_PREPARE_SECRET_DELIVERY_STATUS_OK,
		Token:       token,
		PublicKey:   publicKey,
		KeyType:     rcsecretsv1.KeyType_KEY_TYPE_NACL_SEALED_BOX_X25519,
		KeyValidity: durationpb.New(validity),
	}, nil
}

func statusResponse(status rcsecretsv1.PrepareSecretDeliveryStatus) *rcsecretsv1.PrepareSecretDeliveryResponse {
	return &rcsecretsv1.PrepareSecretDeliveryResponse{Status: status}
}
