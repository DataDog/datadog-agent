// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package file

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

// FingerprinterMock is a mock implementation of the Fingerprinter interface
type FingerprinterMock struct {
	shouldFileFingerprint map[string]bool
	fingerprints          map[string]*types.Fingerprint
}

// NewFingerprinterMock creates a new FingerprintMock
func NewFingerprinterMock() *FingerprinterMock {
	return &FingerprinterMock{
		shouldFileFingerprint: make(map[string]bool),
		fingerprints:          make(map[string]*types.Fingerprint),
	}
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
	f.fingerprints[filepath] = fingerprint
}

// SetInvalidFingerprint sets an invalid fingerprint for the given file
func (f *FingerprinterMock) SetInvalidFingerprint(filepath string) {
	f.shouldFileFingerprint[filepath] = true
	f.fingerprints[filepath] = &types.Fingerprint{Value: types.InvalidFingerprintValue, Config: nil}
}

// ComputeFingerprint returns previously set fingerprint for the given file, or an error if no fingerprint was set
func (f *FingerprinterMock) ComputeFingerprint(file *File) (*types.Fingerprint, error) {
	if fingerprint, ok := f.fingerprints[file.Path]; ok {
		return fingerprint, nil
	}
	return nil, fmt.Errorf("no fingerprint set for file %s", file.Path)
}

// ComputeFingerprintFromConfig returns previously set fingerprint for the given file path, or an error if no fingerprint was set
func (f *FingerprinterMock) ComputeFingerprintFromConfig(filepath string, _ *types.FingerprintConfig) (*types.Fingerprint, error) {
	if fingerprint, ok := f.fingerprints[filepath]; ok {
		return fingerprint, nil
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
