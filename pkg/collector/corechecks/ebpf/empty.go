// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux || !cgo
// +build !linux !cgo

package ebpf

import "github.com/DataDog/datadog-agent/pkg/traceinit"

// Avoid the following error on non-supported platforms:
// "build constraints exclude all Go files in github.com\DataDog\datadog-agent\pkg\collector\corechecks\ebpf"
func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\collector\corechecks\ebpf\empty.go 13`)
}