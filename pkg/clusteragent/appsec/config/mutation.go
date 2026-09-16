// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package config

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NormalizeOutcome canonicalizes mutation outcomes into a metrics-safe outcome, reason, and retained error.
func NormalizeOutcome(o MutationOutcome, err error) (MutationOutcome, string, error) {
	switch o {
	case MutationMutated:
		if err != nil {
			return MutationError, "error", err
		}
		return MutationMutated, "none", nil
	case MutationSkipped:
		if err == nil {
			log.Warnf("mutation skipped with nil error")
			return MutationSkipped, string(SkipReasonUnknown), nil
		}

		var skipped *MutationSkippedReason
		if errors.As(err, &skipped) {
			return MutationSkipped, string(skipped.Reason), nil
		}
		return MutationError, "error", err
	case MutationError:
		if err == nil {
			log.Warnf("mutation error with nil error")
			return MutationError, "error", errors.New("mutation error with nil error")
		}
		return MutationError, "error", err
	default:
		return MutationError, "error", fmt.Errorf("unexpected mutation outcome %v", o)
	}
}

// NormalizeOutcomeForAdmission converts a mutation outcome into the legacy admission bool/error contract.
func NormalizeOutcomeForAdmission(o MutationOutcome, err error) (bool, error) {
	canonical, _, retained := NormalizeOutcome(o, err)
	switch canonical {
	case MutationMutated:
		return true, nil
	case MutationSkipped:
		return false, nil
	case MutationError:
		return false, retained
	default:
		return false, fmt.Errorf("unexpected outcome %v", canonical)
	}
}
