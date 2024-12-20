// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// ^^ for now only one check implemented on windows

// Package checks implements the "checks" bundle, for all of the component based agent checks
package checks

import (
	"github.com/DataDog/datadog-agent/comp/checks/agentcrashdetect/agentcrashdetectimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-metrics-logs

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		agentcrashdetectimpl.Module())
}
