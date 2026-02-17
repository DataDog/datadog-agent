// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ddinjector

/*
//! These includes are needed to use constants defined in the ddprocmonapi
#include <WinDef.h>
#include <WinIoCtl.h>

//! Defines the objects used to communicate with the driver as well as its control codes
#include "../include/ddinjector_public.h"
#include <stdlib.h>
*/
import "C"

//const Signature = C.DD_PROCMONDRIVER_SIGNATURE

const (
	GetCountersIOCTL = uint32(C.IOCTL_GET_COUNTERS)
)

type DDInjectorCounterRequest C.struct__COUNTER_REQUEST
type DDInjectorCountersV1 C.struct__DRIVER_COUNTERS_V1

const DDInjectorCounterRequestSize = C.sizeof_struct__COUNTER_REQUEST
const DDInjectorCountersV1Size = C.sizeof_struct__DRIVER_COUNTERS_V1
