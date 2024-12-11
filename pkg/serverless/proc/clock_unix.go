// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package proc

import "github.com/tklauser/go-sysconf"

func getClkTck() (int64, error) {
	clcktck, err := sysconf.Sysconf(sysconf.SC_CLK_TCK)
	return clcktck, err
}
