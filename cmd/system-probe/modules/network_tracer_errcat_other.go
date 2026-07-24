// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build (windows && npm) || darwin

package modules

import (
	networkconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
)

// categorizeTracerError returns err unchanged on platforms where eBPF verifier and USM
// failures cannot occur (Windows, Darwin).
func categorizeTracerError(err error) error {
	return err
}

// checkAndReportUSMState is a no-op on non-Linux platforms where the USM silent-skip
// scenario (CNM starts, USM silently disabled on kernel < 4.14) cannot occur.
func checkAndReportUSMState(_ module.FactoryDependencies, _ *networkconfig.Config) {}
