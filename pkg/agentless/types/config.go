// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package types holds the different types and constants used in the agentless
// scanner. Most of these types are serializable.
package types

// ScannerConfig is the representation of the scan configuration after being
type ScannerConfig struct {
	Env                 string
	DogstatsdPort       int
	DefaultRolesMapping RolesMapping
	AWSRegion           string
	AWSEC2Rate          float64
	AWSEBSListBlockRate float64
	AWSEBSGetBlockRate  float64
	AWSDefaultRate      float64
}
