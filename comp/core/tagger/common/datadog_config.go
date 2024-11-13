// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package common provides common utilities that are useful when interacting with the tagger.
package common

// DatadogConfig contains the configuration specific to Dogstatsd.
type DatadogConfig struct {
	// DogstatsdEntityIDPrecedenceEnabled disable enriching Dogstatsd metrics with tags from "origin detection" when Entity-ID is set.
	DogstatsdEntityIDPrecedenceEnabled bool
	// DogstatsdOptOutEnabled If enabled, and cardinality is none no origin detection is performed.
	DogstatsdOptOutEnabled bool
	// OriginDetectionUnifiedEnabled If enabled, all origin detection mechanisms will be unified to use the same logic.
	OriginDetectionUnifiedEnabled bool
}
