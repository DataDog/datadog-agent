// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build anomalydetectiontestbench

package observerimpl

import "testing"

func TestRealSeasonalScopedSelection(t *testing.T) {
	old := realSeasonalSelectors
	defer func() { realSeasonalSelectors = old }()
	realSeasonalSelectors = []realSeasonalSelector{{Name: "container.net.sent", Tags: []string{"service:backend", "interface:eth0"}}}
	storage := newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 120})
	storage.Add("contract", "container.net.sent", 2, 100, []string{"service:backend", "interface:eth0"})
	meta := realSeasonalTestMeta(t, storage)
	if !realSeasonalEligible(meta) {
		t.Fatal("matching role/metric must be eligible")
	}
	realSeasonalSelectors[0].Tags = []string{"service:other"}
	if realSeasonalEligible(meta) {
		t.Fatal("same metric on another role must remain raw")
	}
	realSeasonalSelectors = nil
	if realSeasonalEligible(meta) {
		t.Fatal("legacy allowlist changed")
	}
}
