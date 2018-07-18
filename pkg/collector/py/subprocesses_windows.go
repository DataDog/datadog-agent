// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython
// +build windows

package py

import (
	"os"
)

func subprocessEnd(proc *os.Process) error {
	return proc.Kill()
}
