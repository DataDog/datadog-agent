// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package tracer

// VPNClassification holds the result of classifying a network interface.
// On non-Windows platforms, this is a stub.
type VPNClassification struct {
	IsVPN bool
}

// VPNClassifier classifies network interfaces as VPN or non-VPN.
// On non-Windows platforms, this is a no-op stub.
type VPNClassifier struct{}

// NewVPNClassifier returns nil on non-Windows platforms
func NewVPNClassifier() *VPNClassifier { return nil }

// Classify always returns a non-VPN result on non-Windows platforms
func (c *VPNClassifier) Classify(_ uint32) VPNClassification { return VPNClassification{} }

// Close is a no-op on non-Windows platforms
func (c *VPNClassifier) Close() {}
