// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pythoninfo defines the host-metadata boundary for Python version data.
package pythoninfo

// NotAvailable is reported in host metadata when this product does not include
// a Python runtime.
const NotAvailable = "n/a"

// Provider reports Python version information for host metadata payloads.
type Provider interface {
	// GetPythonInfo returns the full Python info string.
	GetPythonInfo() string
	// GetPythonVersion returns the short Python version string.
	GetPythonVersion() string
}

type unavailableProvider struct{}

// Unavailable returns a Provider for products that do not include a Python runtime.
func Unavailable() Provider {
	return unavailableProvider{}
}

// WithFallback returns provider when present, or an unavailable Provider otherwise.
func WithFallback(provider Provider) Provider {
	if provider == nil {
		return Unavailable()
	}
	return provider
}

func (unavailableProvider) GetPythonInfo() string {
	return NotAvailable
}

func (unavailableProvider) GetPythonVersion() string {
	return NotAvailable
}
