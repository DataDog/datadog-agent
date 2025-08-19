// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rdnsquerier provides the reverse DNS querier component.
package rdnsquerier

import (
	"context"
)

// team: ndm-integrations

// ReverseDNSResult is the result of a reverse DNS lookup
type ReverseDNSResult struct {
	IP       string
	Hostname string
	Err      error
}

// Component is the component type.
type Component interface {
	GetHostnameAsync([]byte, func(string), func(string, error)) error
	GetHostname(context.Context, string) (string, error)
	GetHostnames(context.Context, []string) map[string]ReverseDNSResult
}
