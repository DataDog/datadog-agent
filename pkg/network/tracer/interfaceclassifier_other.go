// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !darwin

package tracer

// InterfaceClassification holds interface metadata looked up by interface index.
// On platforms other than Windows and Darwin, this is a stub.
type InterfaceClassification struct {
	InterfaceName string
	InterfaceType string
}

// InterfaceClassifier resolves interface indices to interface metadata.
// On platforms other than Windows and Darwin, this is a no-op stub.
type InterfaceClassifier struct{}

// NewInterfaceClassifier returns nil on platforms other than Windows and Darwin
func NewInterfaceClassifier() *InterfaceClassifier { return nil }

// Classify always returns an empty result on platforms other than Windows and Darwin
func (c *InterfaceClassifier) Classify(_ uint32) InterfaceClassification {
	return InterfaceClassification{}
}

// Close is a no-op on platforms other than Windows and Darwin
func (c *InterfaceClassifier) Close() {}
