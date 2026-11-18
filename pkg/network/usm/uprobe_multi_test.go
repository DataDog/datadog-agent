// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package usm

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

func TestUprobeMultiFallback(t *testing.T) {
	for _, probeErr := range []error{ebpf.ErrNotSupported, unix.EPERM} {
		t.Run(probeErr.Error(), func(t *testing.T) {
			multi := &uprobeMulti{enabled: true, roots: []string{"uprobe__foo"}, supportProbe: func() error { return probeErr }}
			mgr := &manager.Manager{}
			opts := &manager.Options{}
			require.NoError(t, multi.BeforeInit(mgr, names.NewModuleName("usm"), opts))
			require.Len(t, mgr.InstructionPatchers, 1)
			require.NoError(t, mgr.InstructionPatchers[0](mgr))
			require.Nil(t, multi.prepared)
			require.NoError(t, multi.AfterInit(mgr, names.NewModuleName("usm"), opts))
			require.Nil(t, multi.loaded, "leave the initialized perf backend selected")
		})
	}
}

// TestUSMUprobeMultiHTTPS exercises the real TLS programs, patched constants,
// shared state maps, and tail-called HTTP decoder. It is opt-in because it
// installs kernel probes and requires the built USM CO-RE assets and Python SSL.
func TestUSMUprobeMultiHTTPS(t *testing.T) {
	if os.Getenv("DD_USM_UPROBE_MULTI_INTEGRATION") != "1" {
		t.Skip("opt-in privileged USM HTTPS integration")
	}
	for _, multi := range []bool{false, true} {
		for _, runtime := range []string{"go", "go-http2", "native"} {
			t.Run(fmt.Sprintf("multi=%t/%s", multi, runtime), func(t *testing.T) {
				cfg := NewUSMEmptyConfig()
				cfg.EnableCORE = true
				cfg.EnableRuntimeCompiler = false
				cfg.AllowRuntimeCompiledFallback = false
				cfg.AllowPrebuiltFallback = false
				cfg.EnableUSMEventStream = false
				cfg.EnableUprobeMulti = multi
				cfg.EnableHTTPMonitoring = true
				cfg.EnableGoTLSSupport = runtime != "native"
				cfg.EnableHTTP2Monitoring = runtime == "go-http2"
				cfg.EnableNativeTLSMonitoring = runtime == "native"
				cfg.GoTLSExcludeSelf = runtime == "go-http2"
				cfg.CollectTCPv4Conns = true
				cfg.CollectTCPv6Conns = true
				monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)
				if multi {
					require.NotNil(t, monitor.ebpfProgram.multiUprobes.loaded, "multi backend must load, not silently fall back")
				} else {
					require.Nil(t, monitor.ebpfProgram.multiUprobes.loaded)
				}
				server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
				server.EnableHTTP2 = runtime == "go-http2"
				server.StartTLS()
				t.Cleanup(server.Close)
				path := "/200/uprobe-multi-" + runtime
				url := server.URL + path
				if runtime == "go-http2" {
					path = "/200/request-0"
					client := os.Getenv("DD_USM_UPROBE_MULTI_CLIENT")
					require.NotEmpty(t, client, "provide the Bazel-built, unstripped Go TLS client")
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					t.Cleanup(cancel)
					command := exec.CommandContext(ctx, client, "-http2", strings.TrimPrefix(server.URL, "https://"), "1")
					command.Stderr = os.Stderr
					input, err := command.StdinPipe()
					require.NoError(t, err)
					require.NoError(t, command.Start())
					t.Cleanup(func() { cancel(); _ = command.Wait() })
					require.NoError(t, GoTLSAttachPID(uint32(command.Process.Pid)))
					_, err = io.WriteString(input, "1")
					require.NoError(t, err)
					require.NoError(t, command.Wait())
				} else if runtime != "native" {
					require.NoError(t, GoTLSAttachPID(uint32(os.Getpid())))
					transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, ForceAttemptHTTP2: runtime == "go-http2"}
					t.Cleanup(transport.CloseIdleConnections)
					client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
					response, err := client.Get(url)
					require.NoError(t, err)
					if runtime == "go-http2" {
						require.Equal(t, 2, response.ProtoMajor)
					}
					_, err = io.Copy(io.Discard, response.Body)
					require.NoError(t, err)
					require.NoError(t, response.Body.Close())
				} else {
					// Import SSL and wait on stdin, so attachment is complete
					// before the client creates its first TLS connection. No
					// application build or timing-based attachment delay needed.
					python, err := exec.LookPath("python3")
					require.NoError(t, err)
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					t.Cleanup(cancel)
					command := exec.CommandContext(ctx, python, "-u", "-c", "import ssl,sys,urllib.request; print('ready'); sys.stdin.read(1); print(urllib.request.urlopen(sys.argv[1], context=ssl._create_unverified_context()).read().decode())", url)
					command.Stderr = os.Stderr
					input, err := command.StdinPipe()
					require.NoError(t, err)
					output, err := command.StdoutPipe()
					require.NoError(t, err)
					require.NoError(t, command.Start())
					t.Cleanup(func() { cancel(); _ = command.Wait() })
					reader := bufio.NewReader(output)
					ready, err := reader.ReadString('\n')
					require.NoError(t, err)
					require.Equal(t, "ready\n", ready)
					err = opensslSpec.Instance.(*sslProgram).attacher.AttachPIDWithOptions(uint32(command.Process.Pid), true)
					require.True(t, err == nil || errors.Is(err, utils.ErrPathIsAlreadyRegistered), "native TLS attach: %v", err)
					_, err = io.WriteString(input, "1")
					require.NoError(t, err)
					body, err := reader.ReadString('\n')
					require.NoError(t, err)
					require.Equal(t, "ok\n", body)
				}
				// Real kernel events reach the asynchronous HTTP consumer. Poll
				// the resulting payload; this is not a shutdown timing assertion.
				protocol := protocols.HTTP
				if runtime == "go-http2" {
					protocol = protocols.HTTP2
				}
				require.Eventually(t, func() bool {
					for key, stats := range getHTTPLikeProtocolStats(t, monitor, protocol) {
						if key.Path.Content.Get() == path && stats.Data[200] != nil && stats.Data[200].Count > 0 {
							return true
						}
					}
					return false
				}, 10*time.Second, 20*time.Millisecond, "real %s HTTPS request must reach USM via backend multi=%t", runtime, multi)
				fds, err := os.ReadDir("/proc/self/fd")
				require.NoError(t, err)
				perfFDs := 0
				for _, fd := range fds {
					target, _ := os.Readlink("/proc/self/fd/" + fd.Name())
					if strings.Contains(target, "perf_event") {
						perfFDs++
					}
				}
				start := time.Now()
				monitor.Stop()
				t.Logf("real USM shutdown: runtime=%s multi=%t perf_fds=%d seconds=%.6f", runtime, multi, perfFDs, time.Since(start).Seconds())

			})
		}
	}
}
