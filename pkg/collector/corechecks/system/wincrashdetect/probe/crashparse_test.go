// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.
//go:build windows
// +build windows

package probe

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCrashParser(t *testing.T) {

	wcs := &WinCrashStatus{
		FileName: "testdata/crashsample1.txt",
	}
	// first read in the sample data

	readfn = testCrashReader

	parseCrashDump(wcs)

	assert.True(t, wcs.Success)
	assert.Empty(t, wcs.ErrString)
	assert.Equal(t, "Mon Jun 26 20:44:49.742 2023 (UTC - 7:00)", wcs.DateString)
	before, _, _ := strings.Cut(wcs.Offender, "+")
	assert.Equal(t, "ddapmcrash", before)
	assert.Equal(t, "0000007E", wcs.BugCheck)

}
