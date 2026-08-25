// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

// FingerprintResult holds a fingerprint and the open flags actually used to produce it.
type FingerprintResult struct {
	Fingerprint  *types.Fingerprint
	AppliedFlags []types.FileOpenFlag
}

// HasAuthoritativeDirectCandidate reports whether the result is safe to use for Stage 3 verify.
func HasAuthoritativeDirectCandidate(config *types.FingerprintConfig, result FingerprintResult) bool {
	if !types.DirectConfigured(config) {
		return false
	}
	if result.Fingerprint == nil || !result.Fingerprint.ValidFingerprint() {
		return false
	}
	return types.AppliedFlagsIncludeDirect(result.AppliedFlags)
}
