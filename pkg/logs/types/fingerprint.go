// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types provides data structures and types for the logs system.
package types

import (
	"fmt"
)

const (
	// InvalidFingerprintValue is the value that is returned when a fingerprint cannot be produced due to errors
	InvalidFingerprintValue = 0
)

// Fingerprint struct that stores both the value and config used to derive that value
type Fingerprint struct {
	Value     uint64
	Config    *FingerprintConfig
	BytesUsed int // Number of bytes used to compute the fingerprint; fingerprints can be partial (< Config.Count)
}

// String converts the fingerprint to a string
func (f *Fingerprint) String() string {
	return fmt.Sprintf("Fingerprint{Value: %d, BytesUsed: %d, Config: %v}", f.Value, f.BytesUsed, f.Config)
}

// Equals compares two fingerprints and returns true if they are equal
func (f *Fingerprint) Equals(other *Fingerprint) bool {
	return f.Value == other.Value
}

// IsValidFingerprint returns true if the fingerprint is valid (non-zero value and non-nil config)
func (f *Fingerprint) IsValidFingerprint() bool {
	return f.Value != InvalidFingerprintValue && f.Config != nil
}

// IsPartialFingerprint returns true if the fingerprint was computed from partial data
func (f *Fingerprint) IsPartialFingerprint() bool {
	if f.Config == nil {
		return false
	}
	expectedBytes := f.Config.Count
	return f.BytesUsed > 0 && f.BytesUsed < expectedBytes
}

// FingerprintConfig defines the options for the fingerprint configuration.
type FingerprintConfig struct {
	// FingerprintStrategy defines the strategy used for fingerprinting. Options are:
	// - "line_checksum": compute checksum based on line content (default)
	// - "byte_checksum": compute checksum based on byte content
	// - "disabled": disable fingerprinting
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
	// FingerprintStrategyLineChecksum computes a checksum based on line content
	FingerprintStrategyLineChecksum FingerprintStrategy = "line_checksum"
	// FingerprintStrategyByteChecksum computes a checksum based on byte content
	FingerprintStrategyByteChecksum FingerprintStrategy = "byte_checksum"
	// FingerprintStrategyDisabled disables fingerprinting
	FingerprintStrategyDisabled FingerprintStrategy = "disabled"
)

func (s FingerprintStrategy) String() string {
	return string(s)
}

// Validate checks if the fingerprint strategy is valid (either line_checksum or byte_checksum)
func (s FingerprintStrategy) Validate() error {
	switch s {
	case FingerprintStrategyLineChecksum, FingerprintStrategyByteChecksum, FingerprintStrategyDisabled:
		return nil
	}
	return fmt.Errorf("invalid fingerprint strategy: %s", s)
}
