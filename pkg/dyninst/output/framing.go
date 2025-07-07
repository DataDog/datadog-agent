// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build ignore

package output

// #define CGO
// #define uint16_t unsigned short
// #define uint32_t unsigned int
// #define uint64_t unsigned long long
// #include "../ebpf/framing.h"
import "C"

type EventHeader C.di_event_header_t
type DataItemHeader C.di_data_item_header_t
