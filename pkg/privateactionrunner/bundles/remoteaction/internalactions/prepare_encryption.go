// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_internal

import (
	"context"
	"crypto/ecdh"
	"crypto/hpke"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type PrepareEncryptionHandler struct {
	store *encryptioncontext.Store
}

func NewPrepareEncryptionHandler(store *encryptioncontext.Store) *PrepareEncryptionHandler {
	return &PrepareEncryptionHandler{store: store}
}

type PrepareEncryptionInputs struct {
	EncryptionContextID string `json:"encryptionContextId"`
}

type PrepareEncryptionOutputs struct {
	KeyType   string `json:"keyType"`
	PublicKey string `json:"publicKey"`
}

// Run generates an HPKE DHKEM(P-256) key pair and returns the public key.
func (handler *PrepareEncryptionHandler) Run(
	_ context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[PrepareEncryptionInputs](task)
	if err != nil {
		return nil, err
	}
	if inputs.EncryptionContextID == "" {
		return nil, errors.New("encryptionContextId is required")
	}

	privateKey, err := hpke.DHKEM(ecdh.P256()).GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate HPKE key pair: %w", err)
	}

	handler.store.Set(inputs.EncryptionContextID, privateKey)

	return &PrepareEncryptionOutputs{
		KeyType:   encryptioncontext.KeyTypeHPKE,
		PublicKey: base64.StdEncoding.EncodeToString(privateKey.PublicKey().Bytes()),
	}, nil
}
