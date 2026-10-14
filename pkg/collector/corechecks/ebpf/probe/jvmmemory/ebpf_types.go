// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ignore

package jvmmemory

/*
#include "../../c/runtime/jvm-memory-kern-user.h"
*/
import "C"

type ebpfPoolKey C.jvm_pool_key_t
type ebpfPoolValue C.jvm_pool_value_t
type ebpfGCStats C.jvm_gc_stats_t
