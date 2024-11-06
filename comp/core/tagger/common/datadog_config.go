// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package common provides common utilities that are useful when interacting with the tagger.
package common

// DatadogConfig contains the configuration specific to Dogstatsd.
type DatadogConfig struct {
	DogstatsdEntityIDPrecedenceEnabled bool
	DogstatsdOptOutEnabled             bool
	OriginDetectionUnifiedEnabled      bool
}
