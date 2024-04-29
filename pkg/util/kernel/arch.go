// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package kernel is utilities for the Linux kernel
package kernel

import "runtime"

// Arch returns the kernel architecture value, often used within the kernel `include/arch` directory.
func Arch() string {
	// list of GOARCH from https://gist.github.com/asukakenji/f15ba7e588ac42795f421b48b8aede63
	switch runtime.GOARCH {
	case "386", "amd64":
		return "x86"
	case "arm":
		return "arm"
	case "arm64":
		return "arm64"
	case "ppc64", "ppc64le":
		return "powerpc"
	case "mips", "mipsle", "mips64", "mips64le":
		return "mips"
	case "riscv64":
		return "riscv"
	case "s390x":
		return "s390"
	default:
		return runtime.GOARCH
	}
}
