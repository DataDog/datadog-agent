// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package flare

import (
	"context"
	"errors"
	"testing"
	"time"

	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetErrptData(t *testing.T) {
	mock := flarehelpers.NewFlareBuilderMock(t, false)
	require.NoError(t, getErrptData(context.Background(), mock))

	// The section headers are always present; the errpt output itself depends
	// on the error log content and on running the test as root.
	mock.AssertFileContentMatch(
		`(?s)=+ Agent process deaths by signal.*=+ Kernel paging space events.*=+ Agent SRC subsystem lifecycle.*`,
		"errpt.log")
}

func TestErrptSince(t *testing.T) {
	assert.Equal(t, "1030134526", errptSince(time.Date(2026, 10, 30, 13, 45, 21, 0, time.UTC)))
	// Year rollover: January dates must carry the previous year.
	assert.Equal(t, "0102000027", errptSince(time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)))
}

func TestFilterErrptRecords(t *testing.T) {
	sep := "---------------------------------------------------------------------------\n"
	in := sep +
		"LABEL:          CORE_DUMP\nPROGRAM NAME\nagent-bin\n" + sep +
		"LABEL:          CORE_DUMP\nPROGRAM NAME\nsomeapp\n" + sep +
		"LABEL:          CORE_DUMP\nPROGRAM NAME\ntrace-agent-bin\n"

	out, hadRecords := filterErrptRecords([]byte(in), errptBinaries)
	require.True(t, hadRecords)
	assert.Contains(t, string(out), "agent-bin")
	assert.Contains(t, string(out), "trace-agent-bin")
	assert.NotContains(t, string(out), "someapp")

	_, hadRecords = filterErrptRecords(nil, errptBinaries)
	assert.False(t, hadRecords)
}

func TestBuildErrptLog(t *testing.T) {
	t.Run("with data", func(t *testing.T) {
		content := string(buildErrptLog(
			errptResult{out: []byte("death"), hadRecords: true},
			errptResult{out: []byte("paging"), hadRecords: true},
			errptResult{out: []byte("src"), hadRecords: true},
		))
		assert.Contains(t, content, "=== Agent process deaths by signal")
		assert.Contains(t, content, "=== Kernel paging space events")
		assert.Contains(t, content, "=== Agent SRC subsystem lifecycle")
		assert.NotContains(t, content, "flare --local")
		assert.NotContains(t, content, "errpt failed")
	})

	t.Run("empty and successful yields the root hint", func(t *testing.T) {
		content := string(buildErrptLog(errptResult{}, errptResult{}, errptResult{}))
		assert.Contains(t, content, "sudo datadog-agent flare --local")
	})

	t.Run("failure is reported inline and suppresses the hint", func(t *testing.T) {
		content := string(buildErrptLog(
			errptResult{err: errors.New("permission denied")}, errptResult{}, errptResult{}))
		assert.Contains(t, content, "(errpt failed: permission denied)")
		assert.NotContains(t, content, "sudo datadog-agent flare --local")
	})

	t.Run("filtered-out records are noted without the hint", func(t *testing.T) {
		content := string(buildErrptLog(
			errptResult{hadRecords: true}, errptResult{}, errptResult{}))
		assert.Contains(t, content, "(no matching records)")
		assert.NotContains(t, content, "sudo datadog-agent flare --local")
	})
}

func TestBoundedWriter(t *testing.T) {
	t.Run("under the cap, nothing is lost", func(t *testing.T) {
		w := &boundedWriter{max: 10}
		n, err := w.Write([]byte("12345"))
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, "12345", string(w.Bytes()))
	})

	t.Run("over the cap, keeps the prefix and drains", func(t *testing.T) {
		w := &boundedWriter{max: 10}
		// A single write bigger than the cap must report fully consumed.
		n, err := w.Write([]byte("0123456789ABCDEF"))
		require.NoError(t, err)
		assert.Equal(t, 16, n)
		assert.Equal(t, "0123456789"+truncatedMarker, string(w.Bytes()))

		// Further writes are drained but discarded.
		n, err = w.Write([]byte("more"))
		require.NoError(t, err)
		assert.Equal(t, 4, n)
		assert.Equal(t, "0123456789"+truncatedMarker, string(w.Bytes()))
	})

	t.Run("filling the cap exactly is not truncation", func(t *testing.T) {
		w := &boundedWriter{max: 10}
		_, err := w.Write([]byte("0123456789"))
		require.NoError(t, err)
		assert.Equal(t, "0123456789", string(w.Bytes()))
	})
}
