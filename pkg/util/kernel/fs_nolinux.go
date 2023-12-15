// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package kernel

import "github.com/DataDog/datadog-agent/pkg/util/funcs"

//nolint:revive // TODO(EBPF) Fix revive linter
var ProcFSRoot = funcs.MemoizeNoError(func() string {
	return ""
})

//nolint:revive // TODO(EBPF) Fix revive linter
var SysFSRoot = funcs.MemoizeNoError(func() string {
	return ""
})
