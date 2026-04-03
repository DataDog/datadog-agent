// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package softwareinventoryimpl

import (
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/stretchr/testify/assert"
)

// racyMock wraps a mockSysProbeClient and blocks after the inner mock records
// the call but before returning to the caller. This lets the test thread
// observe the window where GetCallCount() > 0 but cachedInventory is still nil.
type racyMock struct {
	inner       *mockSysProbeClient
	afterRecord chan struct{}
}

func (m *racyMock) GetCheck(module types.ModuleName) ([]software.Entry, error) {
	result, err := m.inner.GetCheck(module)
	<-m.afterRecord
	return result, err
}

func (m *racyMock) GetCallCount() int {
	return m.inner.GetCallCount()
}

// TestCachedInventoryRequiresPayloadSync is a regression test for a race
// condition where WaitForSystemProbe (checking GetCallCount) could return
// before cachedInventory was populated, causing status pages to show
// "0 entries" instead of the actual count.
//
// The fix is to use WaitForPayload (checking SendEventPlatformEvent) which
// guarantees cachedInventory has been written.
//
// Timeline with racyMock (blocks GetCheck after recording the call):
//
//	Goroutine                              Test thread
//	─────────                              ───────────
//	GetCheck() → inner mock records call
//	             (GetCallCount() = 1)
//	<blocked on afterRecord channel>       GetCallCount() > 0 ✓
//	                                       reads cachedInventory → nil → "0 entries"
//	<test closes afterRecord>
//	GetCheck returns → writes cachedInventory
//	                                       SendEventPlatformEvent called ✓
//	                                       reads cachedInventory → data → "2 entries"
func TestCachedInventoryRequiresPayloadSync(t *testing.T) {
	afterRecord := make(chan struct{})
	f := newFixtureWithData(t, true, []software.Entry{
		{DisplayName: "FooApp", ProductCode: "foo", Source: "app"},
		{DisplayName: "BarApp", ProductCode: "bar", Source: "pkg"},
	})

	f.sysProbeClient = &racyMock{
		inner:       f.sysProbeClientAsMock(),
		afterRecord: afterRecord,
	}

	sut := f.sut()

	// ── Phase 1: WaitForSystemProbe does NOT guarantee data is cached ────
	sut.WaitForSystemProbe()

	// GetCheck was called and recorded, but hasn't returned yet (blocked by racyMock).
	// cachedInventory is still nil, so the status page shows 0 entries.
	var buf bytes.Buffer
	err := sut.HTML(false, &buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "<strong>Summary:</strong> 0 entries",
		"cachedInventory must be nil while GetCheck hasn't returned")

	// ── Phase 2: WaitForPayload DOES guarantee data is cached ────────────
	close(afterRecord)
	sut.WaitForPayload()

	buf.Reset()
	err = sut.HTML(false, &buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "<strong>Summary:</strong> 2 entries",
		"cachedInventory must be populated after payload is sent")
}
