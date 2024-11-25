// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// This doesn't need BPF, but it's built with this tag to only run with
// system-probe tests, otherwise linters show an error for the core agent tests.
//go:build linux_bpf

package module

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// TestShouldIgnorePid check cases of ignored and non-ignored services
func TestShouldIgnorePid(t *testing.T) {
	testCases := []struct {
		name    string
		comm    string
		service string
		ignore  bool
	}{
		{
			name:    "should ignore datadog agent service",
			comm:    "agent",
			service: "datadog-agent",
			ignore:  true,
		},
		{
			name:    "should not ignore dummy service",
			comm:    "dummy",
			service: "dummy",
			ignore:  false,
		},
	}

	serverBin := buildTestBin(t)
	serverDir := filepath.Dir(serverBin)

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(func() {
				cancel()
				os.Remove(test.comm)
			})

			makeAlias(t, test.comm, serverBin)
			bin := filepath.Join(serverDir, test.comm)
			cmd := exec.CommandContext(ctx, bin)
			cmd.Env = append(cmd.Environ(), "DD_SERVICE="+test.service)

			require.NoError(t, cmd.Start())
			t.Cleanup(func() {
				_ = cmd.Process.Kill()
			})

			discovery := newDiscovery()
			require.NotEmpty(t, discovery)

			proc, err := customNewProcess(int32(cmd.Process.Pid))
			require.NoError(t, err)

			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				// wait until the service name becomes available
				info, err := discovery.getServiceInfo(proc)
				assert.NoError(collect, err)
				assert.Equal(collect, test.service, info.ddServiceName)
			}, 3*time.Second, 100*time.Millisecond)

			// now can check the ignored service
			discoveryCtx := parsingContext{
				procRoot:  kernel.ProcFSRoot(),
				netNsInfo: make(map[uint32]*namespaceInfo),
			}
			service := discovery.getService(discoveryCtx, int32(cmd.Process.Pid))
			if test.ignore {
				require.Empty(t, service)
			} else {
				require.NotEmpty(t, service)
			}

			// check saved pid to ignore
			ignore := discovery.shouldIgnorePid(proc.Pid)
			require.Equal(t, test.ignore, ignore)
		})
	}
}
