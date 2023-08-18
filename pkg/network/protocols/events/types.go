// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package events

/*
#include "../../ebpf/c/protocols/events-types.h"
*/
import "C"

type batch C.batch_data_t
type batchKey C.batch_key_t

const (
	batchPagesPerCPU = C.BATCH_PAGES_PER_CPU
	batchBufferSize  = C.BATCH_BUFFER_SIZE
)
