// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package mock provides a mock implementation of the delegatedauth component for testing
package mock

import (
	"context"
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
)

// Mock is a mock implementation of the delegatedauth.Component interface
type Mock struct {
	GetWorkloadTokenFunc func(context.Context, string, common.WorkloadTokenRequest) (string, error)
	AddInstanceFunc      func(context.Context, delegatedauth.InstanceParams) error
}

var _ delegatedauth.Component = (*Mock)(nil)

// Provides is the mock component output
type Provides struct {
	Comp delegatedauth.Component
}

// New creates a new mock delegatedauth component for testing
func New(_ testing.TB) delegatedauth.Component {
	return &Mock{}
}

// AddInstance calls the mock function if set, otherwise returns nil
func (m *Mock) AddInstance(ctx context.Context, params delegatedauth.InstanceParams) error {
	if m.AddInstanceFunc != nil {
		return m.AddInstanceFunc(ctx, params)
	}
	return nil
}

// GetWorkloadToken calls the mock function if set, otherwise reports unavailable.
func (m *Mock) GetWorkloadToken(ctx context.Context, key string, request common.WorkloadTokenRequest) (string, error) {
	if m.GetWorkloadTokenFunc != nil {
		return m.GetWorkloadTokenFunc(ctx, key, request)
	}
	return "", errors.New("workload tokens are unavailable in the delegated auth mock")
}
