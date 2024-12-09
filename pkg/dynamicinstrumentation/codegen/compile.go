// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package codegen

//go:generate $GOPATH/bin/include_headers pkg/dynamicinstrumentation/codegen/c/dynamicinstrumentation.c pkg/ebpf/bytecode/build/runtime/dynamicinstrumentation.c pkg/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/dynamicinstrumentation.c pkg/ebpf/bytecode/runtime/dynamicinstrumentation.go runtime
