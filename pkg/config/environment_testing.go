// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package config

// Setting a default list of features with what is widely used in unit tests.
func init() {
	detectedFeatures = FeatureMap{Docker: struct{}{}}
}

func SetFeature(feature Feature) {
	featureLock.Lock()
	defer featureLock.Unlock()

	detectedFeatures[feature] = struct{}{}
}

func ClearFeatures() {
	featureLock.Lock()
	defer featureLock.Unlock()

	detectedFeatures = make(FeatureMap)
}
