// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package lifecycle

import (
	"os"
	"syscall"
)

// RunSignal is sent to the child on /run. SIGUSR2 lets tracer libraries
// (e.g. dd-trace-js) reseed their PRNG after a Firecracker clone restore.
var RunSignal os.Signal = syscall.SIGUSR2
