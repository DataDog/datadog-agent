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
	shouldFileFingerprint  map[string]bool
	fingerprints           map[string]*fingerprintStore
	handleFingerprints     map[string]*types.Fingerprint
	computeErrors          map[string]error
	computeResultCallCount int
	computeFromHandleCalls int
	computeFromConfigCalls int
}

// NewFingerprinterMock creates a new FingerprintMock
func NewFingerprinterMock() *FingerprinterMock {
	return &FingerprinterMock{
		shouldFileFingerprint: make(map[string]bool),
		fingerprints:          make(map[string]*fingerprintStore),
		handleFingerprints:    make(map[string]*types.Fingerprint),
		computeErrors:         make(map[string]error),
	}
}

type fingerprintStore struct {
	idx             int
	fingerprints    []*types.Fingerprint
	Config          *types.FingerprintConfig
	appliedFlags    []types.FileOpenFlag
	hasAppliedFlags bool
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

// SetFingerprintWithAppliedFlags sets the fingerprint and explicit applied open flags for a file.
func (f *FingerprinterMock) SetFingerprintWithAppliedFlags(filepath string, fingerprint *types.Fingerprint, config *types.FingerprintConfig, appliedFlags []types.FileOpenFlag) {
	f.shouldFileFingerprint[filepath] = true
	f.fingerprints[filepath] = &fingerprintStore{
		fingerprints:    []*types.Fingerprint{fingerprint},
		Config:          config,
		appliedFlags:    append([]types.FileOpenFlag(nil), appliedFlags...),
		hasAppliedFlags: true,
	}
}

// SetHandleFingerprint sets the fingerprint returned by ComputeFingerprintFromHandle for a path.
func (f *FingerprinterMock) SetHandleFingerprint(filepath string, fingerprint *types.Fingerprint) {
	f.handleFingerprints[filepath] = fingerprint
}

// ResetCallCounts clears mock invocation counters.
func (f *FingerprinterMock) ResetCallCounts() {
	f.computeResultCallCount = 0
	f.computeFromHandleCalls = 0
	f.computeFromConfigCalls = 0
}

// ComputeResultCallCount returns how many times ComputeFingerprintResult was invoked.
func (f *FingerprinterMock) ComputeResultCallCount() int {
	return f.computeResultCallCount
}

// ComputeFromHandleCallCount returns how many times ComputeFingerprintFromHandle was invoked.
func (f *FingerprinterMock) ComputeFromHandleCallCount() int {
	return f.computeFromHandleCalls
}

// ComputeFromConfigCallCount returns how many times ComputeFingerprintFromConfig was invoked.
func (f *FingerprinterMock) ComputeFromConfigCallCount() int {
	return f.computeFromConfigCalls
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
	result, err := f.ComputeFingerprintResult(file)
	if err != nil {
		return result.Fingerprint, err
	}
	return result.Fingerprint, nil
}

// SetFingerprintError makes ComputeFingerprintResult return err for the given path.
func (f *FingerprinterMock) SetFingerprintError(filepath string, err error) {
	f.computeErrors[filepath] = err
}

// ComputeFingerprintResult returns previously set fingerprint with direct provenance when configured.
func (f *FingerprinterMock) ComputeFingerprintResult(file *File) (FingerprintResult, error) {
	f.computeResultCallCount++
	if err, ok := f.computeErrors[file.Path]; ok && err != nil {
		return FingerprintResult{}, err
	}
	if store, ok := f.fingerprints[file.Path]; ok {
		fp := store.Next()
		var applied []types.FileOpenFlag
		if store.hasAppliedFlags {
			applied = append([]types.FileOpenFlag(nil), store.appliedFlags...)
		} else if store.Config != nil && types.DirectConfigured(store.Config) {
			applied = []types.FileOpenFlag{types.FileOpenFlagDirect}
		}
		return FingerprintResult{Fingerprint: fp, AppliedFlags: applied}, nil
	}
	return FingerprintResult{}, fmt.Errorf("no fingerprint set for file %s", file.Path)
}

// ComputeFingerprintFromConfig returns previously set fingerprint for the given file path, or an error if no fingerprint was set
func (f *FingerprinterMock) ComputeFingerprintFromConfig(filepath string, _ *types.FingerprintConfig) (*types.Fingerprint, error) {
	f.computeFromConfigCalls++
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
	f.computeFromHandleCalls++
	if fp, ok := f.handleFingerprints[osFile.Name()]; ok {
		return fp, nil
	}
	if store, ok := f.fingerprints[osFile.Name()]; ok {
		return store.Next(), nil
	}
	return nil, fmt.Errorf("no fingerprint set for file %s", osFile.Name())
}

// ForgetOpenFlagsUnsupported is a no-op for the mock implementation.
func (f *FingerprinterMock) ForgetOpenFlagsUnsupported(...string) {}
