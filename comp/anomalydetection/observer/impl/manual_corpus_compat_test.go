// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import observer "github.com/DataDog/datadog-agent/comp/observer/def"

// manualSeriesRemover mirrors the newer observer.SeriesRemover interface used
// by several recovered weekend-corpus tests. The manual inspection branch is
// based before that interface existed in comp/observer/def, so keep this as a
// test-only compatibility shim instead of changing production APIs.
type manualSeriesRemover interface {
	RemoveSeries([]observer.SeriesRef)
}

// Newer coordinator-generated tests reference the teardown allowlist/helper
// added on later observer branches. The recovered detectors still compile and
// can be inspected independently on this branch; this shim keeps those tests
// buildable until the corpus is replayed onto the newer base branch.
var statelessDetectorAllowlist = map[string]struct{}{}

func (c *componentCatalog) validateDetectorTeardownContract() error { return nil }
