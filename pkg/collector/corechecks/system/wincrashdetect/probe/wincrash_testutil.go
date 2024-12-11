// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test && windows

package probe

type readCrashDumpType func(filename string, ctx *logCallbackContext, _ *uint32) error
type parseCrashDumpType func(wcs *WinCrashStatus)

// SetCachedSettings sets the settings used for tests without reading the Registry.
func (p *WinCrashProbe) SetCachedSettings(wcs *WinCrashStatus) {
	p.status = wcs
}

// OverrideCrashDumpReader relpaces the crash dump reading function for tests.
func OverrideCrashDumpReader(customCrashReader readCrashDumpType) {
	readfn = customCrashReader
}

// OverrideCrashDumpParser relpaces the crash dump parsing function for tests.
func OverrideCrashDumpParser(customParseCrashDump parseCrashDumpType) {
	parseCrashDump = customParseCrashDump
}
