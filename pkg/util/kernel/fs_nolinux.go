// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package kernel

import "github.com/DataDog/datadog-agent/pkg/util/funcs"

// ProcFSRoot is the path to procfs
var ProcFSRoot = funcs.MemoizeNoError(func() string {
	return ""
})

// SysFSRoot is the path to sysfs
var SysFSRoot = funcs.MemoizeNoError(func() string {
	return ""
})
