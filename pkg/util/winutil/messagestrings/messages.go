// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package messagestrings defines the MESSAGETABLE constants used by agent binaries
package messagestrings

//#include "messagestrings.h"
import "C"

//revive:disable:var-naming Name is intended to match the Windows const name

// MESSAGETABLE constants used for formatting messages
const (
	MSG_AGENT_START_FAILURE         = C.MSG_AGENT_START_FAILURE
	MSG_SERVICE_FAILED              = C.MSG_SERVICE_FAILED
	MSG_SERVICE_STARTED             = C.MSG_SERVICE_STARTED
	MSG_SERVICE_STARTING            = C.MSG_SERVICE_STARTING
	MSG_SERVICE_STOPPED             = C.MSG_SERVICE_STOPPED
	MSG_RECEIVED_STOP_SVC_COMMAND   = C.MSG_RECEIVED_STOP_SVC_COMMAND
	MSG_SYSPROBE_RESTART_INACTIVITY = C.MSG_SYSPROBE_RESTART_INACTIVITY
	MSG_UNEXPECTED_CONTROL_REQUEST  = C.MSG_UNEXPECTED_CONTROL_REQUEST
	MSG_WARN_CONFIGUPGRADE_FAILED   = C.MSG_WARN_CONFIGUPGRADE_FAILED
	MSG_WARNING_PROGRAMDATA_ERROR   = C.MSG_WARNING_PROGRAMDATA_ERROR
)

//revive:enable:var-naming
