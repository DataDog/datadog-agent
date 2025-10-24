// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package delegatedauth implements synchronizing the delegated auth API key throughout the agent configuration
package delegatedauth

import (
	"context"
)

// Component to handle delegated auth retrieval and propagation of API keys
type Component interface {
	GetAPIKey(ctx context.Context) (*string, error)
	RefreshAPIKey(ctx context.Context) error
}
