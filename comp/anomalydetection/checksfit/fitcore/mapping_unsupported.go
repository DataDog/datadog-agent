// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && (!darwin || !cgo)

package fitcore

// This file keeps the transport's shared-memory mapping compiling on platforms
// without POSIX shared memory. Every entry point fails with ErrUnsupported, so
// callers must check checksfit.Supported before relying on FIT.

// mapping stands in for the shared-memory mapping on unsupported platforms.
type mapping struct {
	fd       int
	region   []byte
	name     string
	capacity int
}

// word reports that this platform has no FIT implementation.
func (m *mapping) word(int) *uint32 { return nil }

// ring reports that this platform has no FIT implementation.
func (m *mapping) ring() []byte { return nil }

// unlinkName reports that this platform has no FIT implementation.
func (m *mapping) unlinkName() error { return errUnsupported }

// close reports that this platform has no FIT implementation.
func (m *mapping) close() error { return errUnsupported }

// createMapping reports that this platform has no FIT implementation.
func createMapping(uint64, int, uint32) (*mapping, string, error) { return nil, "", errUnsupported }

// openMapping reports that this platform has no FIT implementation.
func openMapping(string, uint64, int, uint32) (*mapping, error) { return nil, errUnsupported }

// mapShared reports that this platform has no FIT implementation.
func mapShared(int, string, int) (*mapping, error) { return nil, errUnsupported }

// writeMetadata is unreachable on this platform; the mapping never exists.
func writeMetadata([]byte, uint64, int, uint32) {}

// anyNonZero reports that this platform has no FIT implementation.
func anyNonZero([]byte) bool { return true }
