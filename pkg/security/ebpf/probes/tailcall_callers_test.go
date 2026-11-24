// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFentryTailCallRouteCallers makes sure that the tail call route callers stay in sync with the probes and routes,
// a stale entry would silently drop events in fentry mode
func TestFentryTailCallRouteCallers(t *testing.T) {
	programs := make(map[string]bool)
	for _, probe := range AllProbes(true, "") {
		programs[probe.EBPFFuncName] = true
	}

	routes := AllTailRoutes(true, true, true, true, true)
	for _, route := range routes {
		programs[route.ProbeIdentificationPair.EBPFFuncName] = true
	}

	for _, rc := range AllFentryTailCallRouteCallers() {
		found := false
		for _, route := range routes {
			if route.ProgArrayName == rc.ProgArrayName && route.Key == rc.Key {
				found = true
				break
			}
		}
		assert.True(t, found, "no tail call route for key %d of %s", rc.Key, rc.ProgArrayName)

		assert.NotEmpty(t, rc.Callers, "no caller for key %d of %s", rc.Key, rc.ProgArrayName)
		for _, caller := range rc.Callers {
			assert.True(t, programs[caller], "unknown caller %s for key %d of %s", caller, rc.Key, rc.ProgArrayName)
		}
	}
}
