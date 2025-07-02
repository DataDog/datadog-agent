// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build ignore

// Package loader supports setting up the eBPF program.
package loader

// #define CGO
// #define bool _Bool
// #define int64_t long long
// #define uint8_t unsigned char
// #define uint16_t unsigned short
// #define uint32_t unsigned int
// #define uint64_t unsigned long long
// #include "../ebpf/types.h"
import "C"

type typeInfo C.type_info_t
type probeParams C.probe_params_t
type throttlerParams C.throttler_params_t
