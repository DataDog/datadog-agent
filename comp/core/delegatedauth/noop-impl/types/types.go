// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types provides the types for the noop implementation of the delegated auth component
package types

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
)

// DelegatedAuthNoop is a noop implementation of the delegated auth component
type DelegatedAuthNoop struct{}

var _ delegatedauth.Component = (*DelegatedAuthNoop)(nil)

// AddInstance does nothing in the noop implementation
func (r *DelegatedAuthNoop) AddInstance(_ context.Context, _ delegatedauth.InstanceParams) error {
	return nil
}

// GetWorkloadAuthorization fails closed when delegated authentication is unavailable.
func (r *DelegatedAuthNoop) GetWorkloadAuthorization(context.Context, string, string) (*common.WorkloadAuthorization, error) {
	return nil, common.ErrWorkloadAuthorizationUnavailable
}
