// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"fmt"
	_ "net/http/pprof"
	"os"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
)

func TestRunCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"run"},
		run,
		func() {})
}

func TestSocketPathNotWritableClassification(t *testing.T) {
	for _, errno := range []syscall.Errno{syscall.EROFS, syscall.EACCES, syscall.EPERM} {
		t.Run(errno.Error(), func(t *testing.T) {
			err := fmt.Errorf("uds: listen: %w", &os.PathError{
				Op:   "bind",
				Path: "/readonly/sysprobe.sock",
				Err:  errno,
			})

			assert.True(t, isSocketPathNotWritable(err))
			assert.ErrorIs(t, err, errno)
			assert.True(t, isSocketPathNotWritable(fmt.Errorf("failed to create listener: %w", err)))
		})
	}
}

func TestSocketPathNotWritableClassificationExcludesOtherSocketErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "address in use",
			err: &os.SyscallError{
				Syscall: "bind",
				Err:     syscall.EADDRINUSE,
			},
		},
		{
			name: "invalid argument",
			err: &os.SyscallError{
				Syscall: "bind",
				Err:     syscall.EINVAL,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := fmt.Errorf("uds: listen: %w", tc.err)
			assert.False(t, isSocketPathNotWritable(err))
		})
	}
}
