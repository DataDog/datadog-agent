// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package flare

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
	"time"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// svmonTimeout bounds the `svmon -P` invocation.
const svmonTimeout = 10 * time.Second

// getSvmonData captures the AIX virtual memory segment breakdown (heap,
// stack, text, shared library segments) for the running Agent process, since
// resource limits are enforced per segment and RSS/VSZ alone don't show which
// segment is close to its ulimit.
func getSvmonData(ctx context.Context, fb flaretypes.FlareBuilder) error {
	cancelctx, cancelfunc := context.WithTimeout(ctx, svmonTimeout)
	defer cancelfunc()

	pid := strconv.Itoa(os.Getpid())
	cmd := exec.CommandContext(cancelctx, "svmon", "-P", pid, "-O", "summary=basic,unit=KB")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		log.Errorf("error running svmon -P %s: %s", pid, err)
	}

	return fb.AddFile("svmon.log", out.Bytes())
}
