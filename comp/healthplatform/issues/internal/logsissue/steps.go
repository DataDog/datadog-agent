// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsissue

import "github.com/DataDog/agent-payload/v5/healthplatform"

// Steps turns remediation texts into proto steps numbered from 1, in order, which
// is what the component's rules require (contiguous, 1-indexed).
func Steps(texts ...string) []*healthplatform.RemediationStep {
	steps := make([]*healthplatform.RemediationStep, len(texts))
	for i, text := range texts {
		steps[i] = &healthplatform.RemediationStep{Order: int32(i + 1), Text: text}
	}
	return steps
}
