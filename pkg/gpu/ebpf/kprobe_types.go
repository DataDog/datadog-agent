// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build ignore || generate

//go:generate go run golang.org/x/tools/cmd/stringer@latest -output kprobe_types_string_linux.go -type=CudaEventType -linecomment

package ebpf

/*
#include "./c/types.h"
*/
import "C"

type CudaEventType C.cuda_event_type_t
type CudaEventHeader C.cuda_event_header_t

type CudaKernelLaunch C.cuda_kernel_launch_t
type Dim3 C.dim3

type CudaSync C.cuda_sync_t

type CudaMemEvent C.cuda_memory_event_t
type CudaMemEventType C.cuda_memory_event_type_t

type CudaSetDeviceEvent C.cuda_set_device_event_t

type CudaVisibleDevicesSetEvent C.cuda_visible_devices_set_t

type CudaEventKey C.cuda_event_key_t
type CudaEventValue C.cuda_event_value_t

const CudaEventTypeKernelLaunch CudaEventType = C.cuda_kernel_launch
const CudaEventTypeMemory CudaEventType = C.cuda_memory_event
const CudaEventTypeSync CudaEventType = C.cuda_sync
const CudaEventTypeSetDevice CudaEventType = C.cuda_set_device
const CudaEventTypeVisibleDevicesSet CudaEventType = C.cuda_visible_devices_set
const CudaEventTypeCount CudaEventType = C.cuda_event_type_count

const CudaMemAlloc = C.cudaMalloc
const CudaMemFree = C.cudaFree

const SizeofCudaKernelLaunch = C.sizeof_cuda_kernel_launch_t
const SizeofCudaMemEvent = C.sizeof_cuda_memory_event_t
const SizeofCudaEventHeader = C.sizeof_cuda_event_header_t
const SizeofCudaSync = C.sizeof_cuda_sync_t
const SizeofCudaSetDeviceEvent = C.sizeof_cuda_set_device_event_t
const SizeofCudaVisibleDevicesSetEvent = C.sizeof_cuda_visible_devices_set_t
