// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && cgo

package output

// This file should only be used for the framing alignment test.

// #define CGO
// #define uint16_t unsigned short
// #define uint32_t unsigned int
// #define uint64_t unsigned long long
// #include "../ebpf/framing.h"
import "C"

// CEventHeader is the message header used for the event program.
type CEventHeader C.event_header_t

// CDataItemHeader is the messages header of a data item.
type CDataItemHeader C.data_item_header_t
