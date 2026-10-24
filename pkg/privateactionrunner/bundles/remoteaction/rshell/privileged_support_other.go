// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin || windows

package com_datadoghq_remoteaction_rshell

import "errors"

// checkPrivilegedHelperSupported reports why this host cannot run the
// privileged helper: it only ships on Linux.
var checkPrivilegedHelperSupported = func() error {
	return errors.New("the privileged helper only runs on Linux")
}
