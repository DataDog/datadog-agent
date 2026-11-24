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

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

// TestUSMUprobeMultiHTTPS exercises the real TLS programs, patched constants,
// shared state maps, and tail-called HTTP decoder. It is opt-in because it
// installs kernel probes and requires the built USM CO-RE assets, Python SSL,
// and Node.js.
func TestUSMUprobeMultiHTTPS(t *testing.T) {
	if os.Getenv("DD_USM_UPROBE_MULTI_INTEGRATION") != "1" {
		t.Skip("opt-in privileged USM HTTPS integration")
	}
	for _, multi := range []bool{false, true} {
		for _, runtime := range []string{"go", "go-http2", "native", "nodejs"} {
			t.Run(fmt.Sprintf("multi=%t/%s", multi, runtime), func(t *testing.T) {
				cfg := NewUSMEmptyConfig()
				cfg.EnableCORE = true
				cfg.EnableRuntimeCompiler = false
				cfg.AllowRuntimeCompiledFallback = false
				cfg.AllowPrebuiltFallback = false
				cfg.EnableUSMEventStream = false
				cfg.EnableUprobeMulti = multi
				cfg.BypassEnabled = true
				cfg.EnableHTTPMonitoring = true
				cfg.EnableGoTLSSupport = runtime == "go" || runtime == "go-http2"
				cfg.EnableHTTP2Monitoring = runtime == "go-http2"
				cfg.EnableNativeTLSMonitoring = runtime == "native"
				cfg.EnableNodeJSMonitoring = runtime == "nodejs"
				cfg.GoTLSExcludeSelf = runtime == "go-http2"
				cfg.CollectTCPv4Conns = true
				cfg.CollectTCPv6Conns = true
				monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)
				enabled, fallback := monitor.ebpfProgram.UprobeMultiStatus()
				require.NoError(t, fallback)
				require.Equal(t, multi, enabled, "exercise the selected backend, not silent fallback")
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
				} else if runtime == "go" {
					require.NoError(t, GoTLSAttachPID(uint32(os.Getpid())))
					transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, ForceAttemptHTTP2: runtime == "go-http2"}
					t.Cleanup(transport.CloseIdleConnections)
					client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
					require.NoError(t, monitor.Pause())
					pausedResponse, err := client.Get(url + "-paused")
					require.NoError(t, err)
					_, err = io.Copy(io.Discard, pausedResponse.Body)
					require.NoError(t, err)
					require.NoError(t, pausedResponse.Body.Close())
					// Resume on a new connection so its TLS state is observed
					// while monitoring is enabled, including with perf probes.
					transport.CloseIdleConnections()
					require.NoError(t, monitor.Resume())
					response, err := client.Get(url)
					require.NoError(t, err)
					if runtime == "go-http2" {
						require.Equal(t, 2, response.ProtoMajor)
					}
					_, err = io.Copy(io.Discard, response.Body)
					require.NoError(t, err)
					require.NoError(t, response.Body.Close())
				} else {
					// Wait on stdin so attachment completes before the client's
					// first TLS connection, without a timing-based delay.
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					t.Cleanup(cancel)
					var command *exec.Cmd
					if runtime == "nodejs" {
						node, err := exec.LookPath("node")
						require.NoError(t, err)
						command = exec.CommandContext(ctx, node, "-e", `const https = require('https'); console.log('ready'); process.stdin.once('data', () => { process.stdin.pause(); https.get(process.argv[1], {rejectUnauthorized: false}, response => { let body = ''; response.on('data', data => body += data); response.on('end', () => console.log(body)); }).on('error', error => { console.error(error); process.exit(1); }); });`, url)
					} else {
						python, err := exec.LookPath("python3")
						require.NoError(t, err)
						command = exec.CommandContext(ctx, python, "-u", "-c", "import ssl,sys,urllib.request; print('ready'); sys.stdin.read(1); print(urllib.request.urlopen(sys.argv[1], context=ssl._create_unverified_context()).read().decode())", url)
					}
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
					attacher := opensslSpec.Instance
					if runtime == "nodejs" {
						attacher = nodejsSpec.Instance
					}
					err = attacher.(multiUprobeProtocol).getUprobeAttacher().AttachPIDWithOptions(uint32(command.Process.Pid), true)
					require.True(t, err == nil || errors.Is(err, utils.ErrPathIsAlreadyRegistered), "%s TLS attach: %v", runtime, err)
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
				capturedWhilePaused := false
				require.Eventually(t, func() bool {
					captured := false
					for key, stats := range getHTTPLikeProtocolStats(t, monitor, protocol) {
						capturedWhilePaused = capturedWhilePaused || key.Path.Content.Get() == path+"-paused"
						if key.Path.Content.Get() == path && stats.Data[200] != nil && stats.Data[200].Count > 0 {
							captured = true
						}
					}
					return captured
				}, 10*time.Second, 20*time.Millisecond, "real %s HTTPS request must reach USM via backend multi=%t", runtime, multi)
				require.False(t, capturedWhilePaused, "paused TLS traffic must not be captured")
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
