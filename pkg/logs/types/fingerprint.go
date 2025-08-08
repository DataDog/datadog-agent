// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"fmt"
)

const (
	InvalidFingerprintValue = 0
)

// Fingerprint struct that stores both the value and config used to derive that value
type Fingerprint struct {
	Value  uint64
	Config *FingerprintConfig
}

func (f *Fingerprint) String() string {
	return fmt.Sprintf("Fingerprint{Value: %d, Config: %v}", f.Value, f.Config)
}

func (f *Fingerprint) Equals(other *Fingerprint) bool {
	return f.Value == other.Value
}

func (f *Fingerprint) ValidFingerprint() bool {
	return f.Value != InvalidFingerprintValue && f.Config != nil
}

// FingerprintConfig defines the options for the fingerprint configuration.
type FingerprintConfig struct {
	// FingerprintStrategy defines the strategy used for fingerprinting. Options are:
	// - "line_checksum": compute checksum based on line content (default)
	// - "byte_checksum": compute checksum based on byte content
	FingerprintStrategy FingerprintStrategy `json:"fingerprint_strategy" mapstructure:"fingerprint_strategy" yaml:"fingerprint_strategy"`

	// Count is the number of lines or bytes to use for fingerprinting, depending on the strategy
	Count int `json:"count" mapstructure:"count" yaml:"count"`

	// CountToSkip is the number of lines or bytes to skip before starting fingerprinting
	CountToSkip int `json:"count_to_skip" mapstructure:"count_to_skip" yaml:"count_to_skip"`

	// MaxBytes is only used for line-based fingerprinting to prevent overloading
	// when reading large files. It's ignored for byte-based fingerprinting.
	MaxBytes int `json:"max_bytes" mapstructure:"max_bytes" yaml:"max_bytes"`
}

// FingerprintStrategy defines the strategy used for fingerprinting
type FingerprintStrategy string

const (
	FingerprintStrategyLineChecksum FingerprintStrategy = "line_checksum"
	FingerprintStrategyByteChecksum FingerprintStrategy = "byte_checksum"
)

// Fingerprinter interface defines the methods for fingerprinting files
type Fingerprinter interface {
	// IsFingerprintingEnabled returns whether or not fingerprinting is enabled
	IsFingerprintingEnabled() bool

	// ComputeFingerprintFromConfig computes a fingerprint for a file path using a specific config
	ComputeFingerprintFromConfig(filepath string, fingerprintConfig *FingerprintConfig) *Fingerprint
}
