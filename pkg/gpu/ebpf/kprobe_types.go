// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package ebpf

/*
#include "./c/types.h"
*/
import "C"

type CudaKernelLaunch C.cuda_kernel_launch_t

const SizeofCudaKernelLaunch = C.sizeof_cuda_kernel_launch_t
