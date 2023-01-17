// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/trace/config/features"
)

// WithFeatures sets the given list of comma-separated features as active and
// returns a function which resets the features to their previous state.
func WithFeatures(feats string) (undo func()) {
	features.Set(feats)
	return func() {
		features.Set(os.Getenv("DD_APM_FEATURES"))
	}
}
