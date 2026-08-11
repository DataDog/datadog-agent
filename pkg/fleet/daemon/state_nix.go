// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package daemon

import "context"

// agentUserErrorCode returns "" off Windows: no other platform runs the Agent as
// an account remote updates can be blocked on, so there is nothing to report.
func agentUserErrorCode(_ context.Context) string {
	return ""
}
