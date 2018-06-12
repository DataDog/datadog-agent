// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package util

import (
	"context"
	"os/exec"
	"time"
)

func getSystemFQDN() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()

	cmd := exec.CommandContext(ctx, "hostname", "-f")

	out, err := cmd.Output()
	return string(out), err
}
