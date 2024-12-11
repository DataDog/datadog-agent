// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package hostname

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

func getSystemFQDN() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/hostname", "-f")

	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
