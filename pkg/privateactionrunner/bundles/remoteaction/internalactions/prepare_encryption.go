// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_internal

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/nacl/box"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

const keyTypeCurve25519 = "curve25519"

type PrepareEncryptionHandler struct {
	store encryptioncontext.Store
}

func NewPrepareEncryptionHandler(store encryptioncontext.Store) *PrepareEncryptionHandler {
	return &PrepareEncryptionHandler{store: store}
}

type PrepareEncryptionInputs struct {
	EncryptionContextID string `json:"encryptionContextId"`
}

type PrepareEncryptionOutputs struct {
	KeyType             string `json:"keyType"`
	PublicKey           string `json:"publicKey"`
	EncryptionContextID string `json:"encryptionContextId"`
}

// Run generates a Curve25519 key pair and returns the public key.
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

	publicKey, privateKey, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Curve25519 key pair: %w", err)
	}

	err = handler.store.Put(inputs.EncryptionContextID, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to store encryption context: %w", err)
	}

	return &PrepareEncryptionOutputs{
		KeyType:             keyTypeCurve25519,
		PublicKey:           base64.StdEncoding.EncodeToString(publicKey[:]),
		EncryptionContextID: inputs.EncryptionContextID,
	}, nil
}
