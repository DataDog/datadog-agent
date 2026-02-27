// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package winutil contains Windows OS utilities
package winutil

import (
	"debug/pe"
	"time"
)

// GetPEBuildTimestamp returns the PE COFF header TimeDateStamp as a UTC time.
// The timestamp reflects the link time set by the toolchain. Some builds may
// zero this value (reproducible builds); in that case, an error is returned.
func GetPEBuildTimestamp(filePath string) (time.Time, error) {
	f, err := pe.Open(filePath)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()
	ts := f.FileHeader.TimeDateStamp
	if ts == 0 {
		return time.Time{}, ErrNoPEBuildTimestamp
	}
	return time.Unix(int64(ts), 0).UTC(), nil
}
