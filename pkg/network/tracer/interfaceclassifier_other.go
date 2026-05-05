// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package tracer

// InterfaceClassification holds interface metadata looked up by interface index.
// On non-Windows platforms, this is a stub.
type InterfaceClassification struct {
	InterfaceName string
	InterfaceType string
}

// InterfaceClassifier resolves interface indices to interface metadata.
// On non-Windows platforms, this is a no-op stub.
type InterfaceClassifier struct{}

// NewInterfaceClassifier returns nil on non-Windows platforms
func NewInterfaceClassifier() *InterfaceClassifier { return nil }

// Classify always returns an empty result on non-Windows platforms
func (c *InterfaceClassifier) Classify(_ uint32) InterfaceClassification {
	return InterfaceClassification{}
}

// Close is a no-op on non-Windows platforms
func (c *InterfaceClassifier) Close() {}
