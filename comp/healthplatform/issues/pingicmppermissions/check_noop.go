// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

// Package pingicmppermissions is a no-op on non-Linux platforms.
package pingicmppermissions

import runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"

func check() ([]runnerdef.IssueReport, error) {
	return nil, nil
}
