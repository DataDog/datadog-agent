// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// features keeps a map of all APM features as defined by the DD_APM_FEATURES
// environment variable at startup.
var features = map[string]struct{}{}

func init() {
	// Whoever imports this package, should have features readily available.
	SetFeatures(os.Getenv("DD_APM_FEATURES"))
}

// SetFeatures sets the given list of comma-separated features as active.
func SetFeatures(feats string) {
	for k := range features {
		delete(features, k)
	}
	all := strings.Split(feats, ",")
	for _, f := range all {
		features[strings.TrimSpace(f)] = struct{}{}
	}
	if active := Features(); len(active) > 0 {
		log.Debugf("Loaded features: %v", active)
	}
}

// HasFeature returns true if the feature f is present. Features are values
// of the DD_APM_FEATURES environment variable.
func HasFeature(f string) bool {
	_, ok := features[f]
	return ok
}

// Features returns a list of all the features configured by means of DD_APM_FEATURES.
func Features() []string {
	var all []string
	for f := range features {
		all = append(all, f)
	}
	return all
}
