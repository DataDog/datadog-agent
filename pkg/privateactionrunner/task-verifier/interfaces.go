// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package taskverifier

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// TaskVerifier unwraps and validates a task received from the OPMS dequeue endpoint.
type TaskVerifier interface {
	UnwrapTask(task *types.Task) (*types.Task, error)
}

// SigningKey is the transport-safe form of a public verification key.
// Key retains the PEM bytes delivered by verified Remote Config.
type SigningKey struct {
	ID      string
	KeyType types.KeyType
	Key     []byte
}

// KeysManager manages the signing keys used to verify task envelopes.
type KeysManager interface {
	Start(ctx context.Context)
	GetKey(keyID string) types.DecodedKey
	WaitForReady(ctx context.Context) error
	Seed(keys []SigningKey) error
	Snapshot() []SigningKey
}
