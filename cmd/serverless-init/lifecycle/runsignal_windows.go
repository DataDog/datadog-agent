// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package lifecycle

import "os"

// RunSignal is a windows placeholder. serverless-init is not supported on
// windows (see cmd/serverless-init/main_windows.go); this declaration exists
// only so the lifecycle package cross-compiles. SIGUSR2 has no windows
// equivalent and this value is never sent.
var RunSignal os.Signal = os.Interrupt
