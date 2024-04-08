// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package types holds the different types and constants used in the agentless
// scanner. Most of these types are serializable.
package types

// ScannerConfig is the representation of the scan configuration after being
type ScannerConfig struct {
	Env                 string       `json:"Env"`
	DogstatsdHost       string       `json:"DogstatsdHost"`
	DogstatsdPort       int          `json:"DogstatsdPort"`
	DefaultRolesMapping RolesMapping `json:"DefaultRolesMapping"`
	DefaultActions      []ScanAction `json:"DefaultActions"`
	DiskMode            DiskMode     `json:"DiskMode"`
	NoForkScanners      bool         `json:"NoForkScanners"`

	AWSRegion           string  `json:"AWSRegion"`
	AWSEC2Rate          float64 `json:"AWSEC2Rate"`
	AWSEBSListBlockRate float64 `json:"AWSEBSListBlockRate"`
	AWSEBSGetBlockRate  float64 `json:"AWSEBSGetBlockRate"`
	AWSDefaultRate      float64 `json:"AWSDefaultRate"`

	AzureClientID string `json:"AzureClientID"`
}
