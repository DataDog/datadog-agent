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

	"github.com/google/uuid"
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
	BoundTaskID string `json:"boundTaskId"`
}

type EncryptionContext struct {
	KeyType             string `json:"keyType"`
	PublicKey           string `json:"publicKey"`
	EncryptionContextID string `json:"encryptionContextId"`
	BoundTaskID         string `json:"boundTaskId"`
}

type PrepareEncryptionOutputs struct {
	KeyType           string            `json:"keyType"`
	EncryptionContext EncryptionContext `json:"encryptionContext"`
}

// Run generates an ephemeral Curve25519 key pair, stashes the private key in
// the shared encryption-context store under (boundTaskId, encryptionContextId),
// and returns the public key plus the lookup identifiers to the caller. A
// subsequent task on the same runner (e.g. testConnectivity) can then retrieve
// the private key from the store to decrypt secret inputs sealed with the
// returned public key.
func (handler *PrepareEncryptionHandler) Run(
	_ context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (any, error) {
	inputs, err := types.ExtractInputs[PrepareEncryptionInputs](task)
	if err != nil {
		return nil, err
	}
	if inputs.BoundTaskID == "" {
		return nil, errors.New("boundTaskId is required")
	}

	publicKey, privateKey, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Curve25519 key pair: %w", err)
	}

	encryptionContextID := uuid.NewString()
	handler.store.Put(inputs.BoundTaskID, encryptionContextID, privateKey)

	return &PrepareEncryptionOutputs{
		KeyType: keyTypeCurve25519,
		EncryptionContext: EncryptionContext{
			KeyType:             keyTypeCurve25519,
			PublicKey:           base64.StdEncoding.EncodeToString(publicKey[:]),
			EncryptionContextID: encryptionContextID,
			BoundTaskID:         inputs.BoundTaskID,
		},
	}, nil
}
