// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf

package modules

import (
	"errors"
	"fmt"

	networkconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
)

// errNetworkProbeUSMUnsupported is reported when USM requires a newer kernel than the one running.
var errNetworkProbeUSMUnsupported = errors.New("USM not supported on this kernel")

// categorizeTracerError wraps err with one of the sentinel errors defined in this package
// so that buildNetworkProbeIssue can select targeted remediation steps.
func categorizeTracerError(err error) error {
	switch {
	case errors.Is(err, usmconfig.ErrNotSupported):
		return fmt.Errorf("%w: %w", errNetworkProbeUSMUnsupported, err)
	default:
		return err
	}
}

// checkAndReportUSMState detects when tracer.NewTracer succeeds but USM was silently
// skipped because the kernel is < 4.14 (CNM can start on 4.4+, USM requires 4.14+).
// Reports a USM issue if USM is enabled but unsupported; resolves any stale USM issue otherwise.
func checkAndReportUSMState(deps module.FactoryDependencies, ncfg *networkconfig.Config) {
	if !ncfg.ServiceMonitoringEnabled {
		resolveNetworkProbeUSMIssue(deps)
		return
	}
	if usmErr := usmconfig.CheckUSMSupported(ncfg); usmErr != nil {
		reportNetworkProbeInitFailure(deps, fmt.Errorf("%w: %w", errNetworkProbeUSMUnsupported, usmErr), ncfg.NPMEnabled, ncfg.ServiceMonitoringEnabled)
	} else {
		resolveNetworkProbeUSMIssue(deps)
	}
}
