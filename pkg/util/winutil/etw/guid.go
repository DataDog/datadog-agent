// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package etw

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// MustParseGUID parses a GUID string (e.g. "{22FB2CD6-0E7B-422B-A0C7-2FAD1FD0E716}").
// Panics if the string is invalid.
func MustParseGUID(s string) windows.GUID {
	g, err := windows.GUIDFromString(s)
	if err != nil {
		panic(fmt.Sprintf("invalid GUID %q: %v", s, err))
	}
	return g
}
