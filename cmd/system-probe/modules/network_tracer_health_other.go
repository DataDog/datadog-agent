// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build (windows && npm) || darwin

package modules

import "github.com/DataDog/datadog-agent/pkg/system-probe/api/module"

// reportNetworkProbeInitFailure is a no-op on Windows and Darwin: health issues for
// eBPF probe init failures are Linux-specific and carry Linux remediation steps.
func reportNetworkProbeInitFailure(_ module.FactoryDependencies, _ error, _, _ bool) {}

// resolveNetworkProbeKernelIssue is a no-op on Windows and Darwin.
func resolveNetworkProbeKernelIssue(_ module.FactoryDependencies) {}
