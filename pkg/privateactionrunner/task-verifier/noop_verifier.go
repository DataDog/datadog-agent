//go:build test

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package taskverifier

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// noOpTaskVerifier bypasses signed envelope validation for e2e tests.
// Tasks are passed through as-is, with inputs read from the outer JSON attributes.
type noOpTaskVerifier struct{}

// NewTaskVerifier returns a no-op TaskVerifier for test builds.
func NewTaskVerifier(_ KeysManager, _ *config.Config) TaskVerifier {
	return &noOpTaskVerifier{}
}

func (n *noOpTaskVerifier) UnwrapTask(task *types.Task) (*types.Task, error) {
	return task, nil
}

// noOpKeysManager satisfies the KeysManager interface without requiring Remote Config.
// WaitForReady returns immediately so PAR does not block on RC key delivery.
type noOpKeysManager struct{}

// NewKeyManager returns a no-op KeysManager for test builds.
func NewKeyManager(_ rcclient.Client) KeysManager {
	return &noOpKeysManager{}
}

func (n *noOpKeysManager) Start(_ context.Context) {}

func (n *noOpKeysManager) GetKey(_ string) types.DecodedKey { return nil }

func (n *noOpKeysManager) WaitForReady() {}
