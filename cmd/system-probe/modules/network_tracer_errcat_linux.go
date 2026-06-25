// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf

package modules

import (
	"errors"
	"fmt"

	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
)

// categorizeTracerError wraps err with one of the sentinel errors defined in network_tracer.go
// so that buildNetworkProbeIssue can select targeted remediation steps.
func categorizeTracerError(err error) error {
	switch {
	case errors.Is(err, usmconfig.ErrNotSupported):
		return fmt.Errorf("%w: %w", errNetworkProbeUSMUnsupported, err)
	default:
		return err
	}
}
