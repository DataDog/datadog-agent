// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rdnsquerierimpl provides the noop rdnsquerier component
package rdnsquerierimpl

import (
	"context"

	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
)

// Provides defines the output of the rdnsquerier component
type Provides struct {
	Comp rdnsquerier.Component
}

type rdnsQuerierImplNone struct{}

// NewNone creates a new noop rdnsquerier component
func NewNone() Provides {
	return Provides{
		Comp: &rdnsQuerierImplNone{},
	}
}

// GetHostnameAsync does nothing for the noop rdnsquerier implementation
func (q *rdnsQuerierImplNone) GetHostnameAsync(_ []byte, _ func(string), _ func(string, error)) error {
	// noop
	return nil
}

// GetHostname does nothing for the noop rdnsquerier implementation
func (q *rdnsQuerierImplNone) GetHostname(_ context.Context, _ string) (string, error) {
	// noop
	return "", nil
}

// GetHostnames does nothing for the noop rdnsquerier implementation
func (q *rdnsQuerierImplNone) GetHostnames(_ context.Context, _ []string) map[string]rdnsquerier.ReverseDNSResult {
	// noop
	return nil
}
