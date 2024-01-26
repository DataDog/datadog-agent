// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package procutil

import (
	"strings"
)
func (ds *DataScrubber) stripArguments(cmdline []string) []string {
	// We will sometimes see the entire command line come in via the first element -- splitting guarantees removal
	// of arguments in these cases.
	if len(cmdline) > 0 {
		return []string{strings.Split(cmdline[0], " ")[0]}
	}
	return cmdline
}
