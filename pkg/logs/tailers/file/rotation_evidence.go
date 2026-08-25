// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

// RotationMethod records how a rotation was detected.
type RotationMethod int

const (
	RotationNone RotationMethod = iota
	RotationFingerprintMismatch
	RotationFilesystemFallback
)

// RotationEvidence holds rotation detection details for sequential handoff.
type RotationEvidence struct {
	Method       RotationMethod
	Fingerprint  *types.Fingerprint
	AppliedFlags []types.FileOpenFlag
}

// RotationCheckResult is returned by CheckRotation.
type RotationCheckResult struct {
	Rotated  bool
	Evidence RotationEvidence
	// BufferedProbeRejected is true when direct is configured but the pathname
	// probe used a buffered open (e.g. CIFS page-cache fallback) instead of O_DIRECT.
	BufferedProbeRejected bool
}

// RotationCheckOptions configures fingerprint-based rotation detection.
type RotationCheckOptions struct {
	// RequireAuthoritativeDirect rejects rotation when direct is configured but
	// the pathname probe did not apply O_DIRECT. Used for sequential handoff.
	RequireAuthoritativeDirect bool
}

// SequentialRotationCheck returns options for Stage 3 sequential handoff.
func SequentialRotationCheck() RotationCheckOptions {
	return RotationCheckOptions{RequireAuthoritativeDirect: true}
}

// ParallelRotationCheck returns options that preserve legacy parallel behavior.
func ParallelRotationCheck() RotationCheckOptions {
	return RotationCheckOptions{RequireAuthoritativeDirect: false}
}

// HasAuthoritativeDirectCandidate reports whether evidence includes a verified direct candidate.
func (e RotationEvidence) HasAuthoritativeDirectCandidate(config *types.FingerprintConfig) bool {
	return HasAuthoritativeDirectCandidate(config, FingerprintResult{
		Fingerprint:  e.Fingerprint,
		AppliedFlags: e.AppliedFlags,
	})
}
