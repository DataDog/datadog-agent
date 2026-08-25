// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CheckRotation detects log rotation and returns candidate fingerprint evidence when available.
func (t *Tailer) CheckRotation(fingerprinter Fingerprinter, opts RotationCheckOptions) (RotationCheckResult, error) {
	result, err := fingerprinter.ComputeFingerprintResult(t.file)
	if err != nil {
		return RotationCheckResult{}, err
	}

	newFingerprint := result.Fingerprint
	effectiveConfig := fingerprinter.GetEffectiveConfigForFile(t.file)

	if newFingerprint != nil && !newFingerprint.ValidFingerprint() {
		log.Debugf("Falling back to filesystem rotation check for %s. New fingerprint invalid", t.file.Path)
		rotated, fsErr := t.DidRotate()
		if fsErr != nil {
			return RotationCheckResult{}, fsErr
		}
		if !rotated {
			return RotationCheckResult{Rotated: false}, nil
		}
		return RotationCheckResult{
			Rotated: true,
			Evidence: RotationEvidence{
				Method: RotationFilesystemFallback,
			},
		}, nil
	}

	if newFingerprint != nil && !t.fingerprint.ValidFingerprint() && newFingerprint.ValidFingerprint() {
		log.Debugf("File rotation detected for %s. Previous fingerprint invalid, new fingerprint valid; assuming rotation", t.file.Path)
		evidence := RotationEvidence{
			Method:       RotationFingerprintMismatch,
			Fingerprint:  newFingerprint,
			AppliedFlags: append([]types.FileOpenFlag(nil), result.AppliedFlags...),
		}
		return t.rotationResultFromFingerprintEvidence(evidence, effectiveConfig, opts), nil
	}

	if newFingerprint != nil && t.fingerprint.ValidFingerprint() && !t.fingerprint.Equals(newFingerprint) {
		log.Debugf("File rotation detected via fingerprint mismatch for %s (old: 0x%x, new: 0x%x)",
			t.file.Path, t.fingerprint.Value, newFingerprint.Value)
		evidence := RotationEvidence{
			Method:       RotationFingerprintMismatch,
			Fingerprint:  newFingerprint,
			AppliedFlags: append([]types.FileOpenFlag(nil), result.AppliedFlags...),
		}
		return t.rotationResultFromFingerprintEvidence(evidence, effectiveConfig, opts), nil
	}

	return RotationCheckResult{Rotated: false}, nil
}

func (t *Tailer) rotationResultFromFingerprintEvidence(
	evidence RotationEvidence,
	effectiveConfig *types.FingerprintConfig,
	opts RotationCheckOptions,
) RotationCheckResult {
	if evidence.HasAuthoritativeDirectCandidate(effectiveConfig) {
		return RotationCheckResult{Rotated: true, Evidence: evidence}
	}
	if opts.RequireAuthoritativeDirect && effectiveConfig != nil && types.DirectConfigured(effectiveConfig) {
		return RotationCheckResult{BufferedProbeRejected: true}
	}
	return RotationCheckResult{Rotated: true, Evidence: evidence}
}
