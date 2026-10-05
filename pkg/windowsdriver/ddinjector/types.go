// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ddinjector

/*
//! These includes are needed to use constants defined in the ddprocmonapi
#include <windef.h>
#include <winioctl.h>

//! Defines the objects used to communicate with the driver as well as its control codes
#include "../include/ddinjector_public.h"
#include <stdlib.h>
*/
import "C"

//const Signature = C.DD_PROCMONDRIVER_SIGNATURE

const (
	GetCountersIOCTL     = uint32(C.IOCTL_GET_COUNTERS)
	GetCapabilitiesIOCTL = uint32(C.IOCTL_GET_DRIVER_CAPABILITIES)

	// CountersVersion1 and CountersVersion2 are the counter contract versions
	// the agent knows how to decode.
	CountersVersion1 = uint32(C.DRIVER_COUNTERS_VERSION_1)
	CountersVersion2 = uint32(C.DRIVER_COUNTERS_VERSION_2)
)

type DDInjectorCounterRequest C.struct__COUNTER_REQUEST
type DDInjectorCapabilities C.struct__DRIVER_CAPABILITIES
type DDInjectorCountersV1 C.struct__DRIVER_COUNTERS_V1
type DDInjectorCountersV2 C.struct__DRIVER_COUNTERS_V2

const DDInjectorCounterRequestSize = C.sizeof_struct__COUNTER_REQUEST
const DDInjectorCapabilitiesSize = C.sizeof_struct__DRIVER_CAPABILITIES
const DDInjectorCountersV1Size = C.sizeof_struct__DRIVER_COUNTERS_V1
const DDInjectorCountersV2Size = C.sizeof_struct__DRIVER_COUNTERS_V2

// NewCounterRequest builds a counter request for the given contract version.
// It lives here (rather than in ddinjector.go) because the RequestedVersion
// field is a C ULONG, which can only be assigned from a Go value inside a cgo
// file.
func NewCounterRequest(version uint32) DDInjectorCounterRequest {
	req := DDInjectorCounterRequest{}
	req.RequestedVersion = C.ulong(version)
	return req
}
