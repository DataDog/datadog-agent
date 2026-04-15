// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package tracer

// InterfaceClassification holds interface metadata and optional VPN classification.
// On non-Windows platforms, this is a stub.
type InterfaceClassification struct {
	InterfaceName string
	InterfaceType string
	IsPhysical    bool
	IsVPN         bool
	VPNName       string
	VPNType       string
}

// VPNClassifier classifies network interfaces as VPN or non-VPN.
// On non-Windows platforms, this is a no-op stub.
type VPNClassifier struct{}

// NewVPNClassifier returns nil on non-Windows platforms
func NewVPNClassifier() *VPNClassifier { return nil }

// Classify always returns an empty result on non-Windows platforms
func (c *VPNClassifier) Classify(_ uint32) InterfaceClassification {
	return InterfaceClassification{}
}

// Close is a no-op on non-Windows platforms
func (c *VPNClassifier) Close() {}
