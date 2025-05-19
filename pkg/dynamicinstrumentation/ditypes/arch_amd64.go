// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && amd64

package ditypes

// StackRegister is the register containing the address of the
// program stack. On x86 DWARF maps the register number 7 to
// the stack pointer.
const StackRegister = 7
