// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package encoding

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"golang.org/x/sys/unix"
)

func formatError(errno int32) model.FailedConnectionReason {
	switch errno {
	case int32(unix.ETIMEDOUT):
		return model.FailedConnectionReason_timedOut
	case int32(unix.ECONNREFUSED):
		return model.FailedConnectionReason_connectionRefused
	default:
		return model.FailedConnectionReason_unknownFailureReason
	}
}
