// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package messagestrings

//#include "messagestrings.h"
import "C"

const (
	MSG_SERVICE_FAILED             = C.MSG_SERVICE_FAILED
	MSG_SERVICE_STARTED            = C.MSG_SERVICE_STARTED
	MSG_SERVICE_STARTING           = C.MSG_SERVICE_STARTING
	MSG_SERVICE_STOPPED            = C.MSG_SERVICE_STOPPED
	MSG_UNEXPECTED_CONTROL_REQUEST = C.MSG_UNEXPECTED_CONTROL_REQUEST
	MSG_RECEIVED_STOP_SVC_COMMAND  = C.MSG_RECEIVED_STOP_SVC_COMMAND
)
