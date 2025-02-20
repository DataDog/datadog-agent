// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

// Package trivy holds the scan components
package trivy

import (
	trivylog "github.com/aquasecurity/trivy/pkg/log"
)

func init() {
	// by default trivy stores all logs until InitLogger is called
	// we call it as soon as possible, asking to disable trivy logs
	trivylog.InitLogger(false, true)
}
