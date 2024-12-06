// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package installer

import (
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
)

// setInstallerUmask no-op on Windows
func setInstallerUmask(_ ddtrace.Span) {}
