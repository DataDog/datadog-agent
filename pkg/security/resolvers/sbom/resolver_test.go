// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package sbom

import (
	"testing"

	"github.com/hashicorp/golang-lru/v2/simplelru"
)

// TestRefreshScanResetsStateForRescan checks that refreshing a workload clears
// its cached SBOM data, resets the SBOM to the pending state, and re-queues it
// for a scan. The state reset matters because analyzeWorkload drops any SBOM
// not in the pending state: a workload is left computed by its initial scan, so
// without the reset the refresh re-scan is silently discarded and the runtime
// properties are never recomputed.
func TestRefreshScanResetsStateForRescan(t *testing.T) {
	dataCache, err := simplelru.NewLRU[workloadKey, *Data](10, nil)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}
	r := &Resolver{
		dataCache: dataCache,
		scanChan:  make(chan *SBOM, 10),
	}

	sbom := NewSBOM("container-id", nil, "image:tag")
	sbom.state.Store(computedState)
	dataCache.Add("image:tag", &Data{})

	r.refreshScan(sbom)

	if got := sbom.state.Load(); got != pendingState {
		t.Errorf("state = %d, want pendingState (%d)", got, pendingState)
	}
	if _, ok := dataCache.Get("image:tag"); ok {
		t.Errorf("cached SBOM data was not invalidated")
	}
	select {
	case queued := <-r.scanChan:
		if queued != sbom {
			t.Errorf("queued unexpected SBOM for re-scan")
		}
	default:
		t.Errorf("workload was not re-queued for a scan")
	}
}
