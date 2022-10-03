// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package encoding

import model "github.com/DataDog/agent-payload/v5/process"

func formatError(errno int32) model.FailedConnectionReason {
	return 0
}
