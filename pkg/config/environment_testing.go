// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package config

import "testing"

// Setting a default list of features with what is widely used in unit tests.
func init() {
	detectionAlwaysDisabledInTests = true
	detectedFeatures = FeatureMap{}
}

// SetFeatures automatically remove feature flags through t.Cleanup
func SetFeatures(t testing.TB, features ...Feature) {
	SetFeaturesNoCleanup(features...)
	t.Cleanup(ClearFeatures)
}

// SetFeaturesNoCleanup DO NOT USE (except in specific integration tests which don't have a testing.T available)
func SetFeaturesNoCleanup(features ...Feature) {
	featureLock.Lock()
	defer featureLock.Unlock()

	detectedFeatures = make(FeatureMap)
	for _, feature := range features {
		detectedFeatures[feature] = struct{}{}
	}
}

// ClearFeatures remove all set features
func ClearFeatures() {
	featureLock.Lock()
	defer featureLock.Unlock()

	detectedFeatures = make(FeatureMap)
}
