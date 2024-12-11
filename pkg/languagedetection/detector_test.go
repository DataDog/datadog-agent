// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"math/rand"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func makeProcess(cmdline []string, comm string) *procutil.Process {
	return &procutil.Process{
		Pid:     rand.Int31(),
		Cmdline: cmdline,
		Comm:    comm,
	}
}
