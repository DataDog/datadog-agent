// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package ebpfcheck

/*
#include "../../c/runtime/ebpf-kern-user.h"
*/
import "C"

type perfBufferKey C.perf_buffer_key_t
type mmapRegion C.mmap_region_t
type ringMmap C.ring_mmap_t
