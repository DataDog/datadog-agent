// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package file

import (
	"fmt"

	"github.com/spf13/afero"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

// FingerprinterMock is a mock implementation of the Fingerprinter interface
type FingerprinterMock struct {
	shouldFileFingerprint map[string]bool
	fingerprints          map[string]*fingerprintStore
}

// NewFingerprinterMock creates a new FingerprintMock
func NewFingerprinterMock() *FingerprinterMock {
	return &FingerprinterMock{
		shouldFileFingerprint: make(map[string]bool),
		fingerprints:          make(map[string]*fingerprintStore),
	}
}

type fingerprintStore struct {
	idx          int
	fingerprints []*types.Fingerprint
	Config       *types.FingerprintConfig
}

func (f *fingerprintStore) Next() *types.Fingerprint {
	if len(f.fingerprints) == 0 {
		return newInvalidFingerprint(nil)
	}
	if f.idx >= len(f.fingerprints) {
		return f.fingerprints[len(f.fingerprints)-1]
	}
	fingerprint := f.fingerprints[f.idx]
	f.idx++
	return fingerprint
}

// SetShouldFileFingerprint sets whether or not the given file should be fingerprinted
func (f *FingerprinterMock) SetShouldFileFingerprint(file *File, shouldFileFingerprint bool) {
	f.shouldFileFingerprint[file.Path] = shouldFileFingerprint
}

// ShouldFileFingerprint returns previously set value for the given file, or false if no value was set
func (f *FingerprinterMock) ShouldFileFingerprint(file *File) bool {
	return f.shouldFileFingerprint[file.Path]
}

// SetFingerprint sets the fingerprint for the given file
func (f *FingerprinterMock) SetFingerprint(filepath string, fingerprint *types.Fingerprint) {
	f.shouldFileFingerprint[filepath] = true
	f.fingerprints[filepath] = &fingerprintStore{
		fingerprints: []*types.Fingerprint{fingerprint},
	}
}

// SetInvalidFingerprint sets an invalid fingerprint for the given file
func (f *FingerprinterMock) SetInvalidFingerprint(filepath string) {
	f.shouldFileFingerprint[filepath] = true
	f.fingerprints[filepath] = &fingerprintStore{
		fingerprints: []*types.Fingerprint{newInvalidFingerprint(nil)},
	}
}

// SetSequence sets a sequence of fingerprints for the given file
func (f *FingerprinterMock) SetSequence(filepath string, fingerprints ...*types.Fingerprint) {
	f.shouldFileFingerprint[filepath] = true
	f.fingerprints[filepath] = &fingerprintStore{
		fingerprints: fingerprints,
	}
}

// ComputeFingerprint returns previously set fingerprint for the given file, or an error if no fingerprint was set
func (f *FingerprinterMock) ComputeFingerprint(file *File) (*types.Fingerprint, error) {
	if store, ok := f.fingerprints[file.Path]; ok {
		return store.Next(), nil
	}
	return nil, fmt.Errorf("no fingerprint set for file %s", file.Path)
}

// ComputeFingerprintFromConfig returns previously set fingerprint for the given file path, or an error if no fingerprint was set
func (f *FingerprinterMock) ComputeFingerprintFromConfig(filepath string, _ *types.FingerprintConfig) (*types.Fingerprint, error) {
	if store, ok := f.fingerprints[filepath]; ok {
		return store.Next(), nil
	}
	return nil, fmt.Errorf("no fingerprint set for file %s", filepath)
}

// GetEffectiveConfigForFile returns nil for the mock implementation
func (f *FingerprinterMock) GetEffectiveConfigForFile(file *File) *types.FingerprintConfig {
	if fingerprint, ok := f.fingerprints[file.Path]; ok && fingerprint.Config != nil {
		return fingerprint.Config
	}
	return nil
}

// ComputeFingerprintFromHandle returns previously set fingerprint for the given File, or an error if no fingerprint was set
func (f *FingerprinterMock) ComputeFingerprintFromHandle(osFile afero.File, _ *types.FingerprintConfig) (*types.Fingerprint, error) {
	if store, ok := f.fingerprints[osFile.Name()]; ok {
		return store.Next(), nil
	}
	return nil, fmt.Errorf("no fingerprint set for file %s", osFile.Name())
}
