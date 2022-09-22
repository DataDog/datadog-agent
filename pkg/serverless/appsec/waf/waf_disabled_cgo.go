// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

// Build when CGO is disabled
//go:build !cgo
// +build !cgo

package waf

var disabledReason = "cgo was disabled during the compilation and should be enabled in order to compile with the waf"
