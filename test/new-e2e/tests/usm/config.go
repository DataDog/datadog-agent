// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package usm contains E2E tests for Universal Service Monitoring
package usm

import _ "embed"

// systemProbeConfigIIS defines the system-probe configuration for IIS HTTP monitoring
//
//go:embed config/usm-iis.yaml
var systemProbeConfigIIS string
