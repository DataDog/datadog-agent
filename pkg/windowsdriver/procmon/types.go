// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build ignore

package procmon

/*
//! These includes are needed to use constants defined in the ddprocmonapi
#include <WinDef.h>
#include <WinIoCtl.h>

//! Defines the objects used to communicate with the driver as well as its control codes
#include "../include/procmonapi.h"
#include <stdlib.h>
*/
import "C"

const Signature = C.DD_PROCMONDRIVER_SIGNATURE

const (
	ProcmonStartIOCTL = C.DD_PROCMONDRIVER_IOCTL_START
	ProcmonStopIOCTL  = C.DD_PROCMONDRIVER_IOCTL_STOP
	ProcmonStatsIOCTL = C.DD_PROCMONDRIVER_IOCTL_GETSTATS

	ProcmonSignature = C.DD_PROCMONDRIVER_SIGNATURE
)

const (
	ProcmonNotifyStop  = C.DD_NOTIFY_STOP
	ProcmonNotifyStart = C.DD_NOTIFY_START
)

type DDProcmonStats C.struct__DD_PROCMON_STATS

type DDProcessNotifyType C.enum__DD_NOTIFY_TYPE
type DDProcessNotification C.struct__DD_PROCESS_NOTIFICATION

const DDProcessNotificationSize = C.sizeof_struct__DD_PROCESS_NOTIFICATION
const DDProcmonStatsSize = C.sizeof_struct__DD_PROCMON_STATS
