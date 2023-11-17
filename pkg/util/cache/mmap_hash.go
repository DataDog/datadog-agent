// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package cache

import (
	"errors"
)

// MaxValueSize is the largest possible value we can store.
const MaxValueSize = 4080

// Stub implementation for mmap_hash for non-Linux platforms.

// Report the mmap hashes in use and any failed checks.
func Report() {
	// Nothing to report on.
}

type mmapHash struct {
}

// Name of a mmapHash.  Based on origin.
func (*mmapHash) Name() string {
	return "unimplemented"
}

func (*mmapHash) lookupOrInsert(key []byte) (string, bool) {
	return string(key), false
}

func (*mmapHash) finalize() {
}

func (*mmapHash) sizes() (int64, int64) {
	return 0, 0
}

func newMmapHash(origin string, fileSize int64, prefixPath string, closeOnRelease bool) (*mmapHash, error) {
	return nil, errors.New("unsupported platform for mmap hash")
}

// Check a string to make sure it's still valid.  Save a histogram of failures for tracking
func Check(tag string) bool {
	return true
}

// CheckDefault checks a string and returns it if it's valid, or returns an indicator of where
// it was called for debugging.
func CheckDefault(tag string) string {
	return tag
}
