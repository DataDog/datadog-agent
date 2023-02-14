// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package ebpf

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	utilkernel "github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// DumpDebugLog is a utility to write the debug log of the running application to a given writer.
// To enable it, you need to enabled BPF DEBUG log in the configuration, and add the following code snippet:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	ebpf.DumpDebugLog(ctx, os.Stdout)
func DumpDebugLog(ctx context.Context, writer io.Writer) error {
	filename := filepath.Base(os.Args[0])
	maxFilenameSize := 15
	if len(filename) < maxFilenameSize {
		maxFilenameSize = len(filename)
	}

	tracefsPath := utilkernel.GetTraceFSMountPath()
	if tracefsPath == "" {
		return errors.New("tracefs not available")
	}
	f, err := os.Open(filepath.Join(tracefsPath, "trace_pipe"))
	if err != nil {
		return err
	}

	go func() {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), filename[0:maxFilenameSize]) {
				_, _ = writer.Write([]byte(scanner.Text() + "\n"))
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()

	return nil
}
