// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build windows

package probe

import (
	"os"

	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testCrashReader(filename string, ctx *logCallbackContext, _ *uint32) error {
	testbytes, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	teststring := string(testbytes)

	logLineCallbackGo(ctx, teststring)
	return nil

}

func testCrashReaderWithLineSplits(filename string, ctx *logCallbackContext, _ *uint32) error {
	testbytes, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	increment := 100
	bodylen := len(testbytes)
	for i := 0; i < bodylen; i += increment {
		left := min(increment, bodylen-i)
		teststring := string(testbytes[i : i+left])
		logLineCallbackGo(ctx, teststring)
	}
	return nil

}

func TestCrashParser(t *testing.T) {

	wcs := &WinCrashStatus{
		FileName: "testdata/crashsample1.txt",
	}
	// first read in the sample data
	OverrideCrashDumpReader(testCrashReader)

	parseCrashDump(wcs)

	assert.Equal(t, WinCrashStatusCodeSuccess, wcs.StatusCode)
	assert.Empty(t, wcs.ErrString)
	assert.Equal(t, "Mon Jun 26 20:44:49.742 2023 (UTC - 7:00)", wcs.DateString)
	before, _, _ := strings.Cut(wcs.Offender, "+")
	assert.Equal(t, "ddapmcrash", before)
	assert.Equal(t, "0000007E", wcs.BugCheck)

}

func TestCrashParserWithLineSplits(t *testing.T) {

	wcs := &WinCrashStatus{
		FileName: "testdata/crashsample1.txt",
	}
	// first read in the sample data

	OverrideCrashDumpReader(testCrashReaderWithLineSplits)

	parseCrashDump(wcs)

	assert.Equal(t, WinCrashStatusCodeSuccess, wcs.StatusCode)
	assert.Empty(t, wcs.ErrString)
	assert.Equal(t, "Mon Jun 26 20:44:49.742 2023 (UTC - 7:00)", wcs.DateString)
	before, _, _ := strings.Cut(wcs.Offender, "+")
	assert.Equal(t, "ddapmcrash", before)
	assert.Equal(t, "0000007E", wcs.BugCheck)

}
