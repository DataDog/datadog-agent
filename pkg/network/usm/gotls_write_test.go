// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package usm

import (
	"crypto/tls"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	gotlstestutil "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/gotls/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/consts"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

// TestGoTLSWriteParsedOncePerCall checks that each Go TLS write is parsed once, including when growing the stack
// restarts the prologue of crypto/tls.(*Conn).Write, which fires its entry probe again.
func TestGoTLSWriteParsedOncePerCall(t *testing.T) {
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		cfg := getDefaultTestConfiguration(true)
		cfg.GoTLSExcludeSelf = false
		if !gotlstestutil.GoTLSSupported(t, cfg) {
			t.Skip("GoTLS not supported for this setup")
		}

		certPath, keyPath, err := testutil.GetCertsPaths()
		require.NoError(t, err)
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		require.NoError(t, err)
		ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = ln.Close() })
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				go func() {
					defer conn.Close()
					buf := make([]byte, 4096)
					for {
						if _, err := conn.Read(buf); err != nil {
							return
						}
					}
				}()
			}
		}()

		monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)
		utils.WaitForProgramsToBeTraced(t, consts.USMModuleName, GoTLSAttacherName, os.Getpid(),
			utils.ManualTracingFallbackEnabled)

		conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{InsecureSkipVerify: true})
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		require.NoError(t, conn.Handshake())

		topic := strings.Repeat("a", 5)
		partition := kmsg.NewProduceRequestTopicPartition()
		recordBatch := makeRecordBatch(makeRecord())
		partition.Records = recordBatch.AppendTo(partition.Records)
		reqTopic := kmsg.NewProduceRequestTopic()
		reqTopic.Topic = topic
		reqTopic.Partitions = append(reqTopic.Partitions, partition)
		req := kmsg.NewProduceRequest()
		req.SetVersion(8)
		req.Acks = 1
		req.TimeoutMillis = 1000
		req.Topics = append(req.Topics, reqTopic)
		request := kmsg.NewRequestFormatter(kmsg.FormatterClientID("kgo")).AppendRequest(nil, &req, 1)

		bucket := func() uint64 {
			telemetry, err := kafka.GetKernelTelemetryMap(monitor.ebpfProgram.Manager.Manager)
			require.NoError(t, err)
			return telemetry.Topic_name_size_buckets[(len(topic)-1)/10]
		}
		before := bucket()

		// Each write starts from a new goroutine's small stack and is made at a growing depth, so that some of them
		// reach crypto/tls.(*Conn).Write right where its prologue has to grow the stack.
		const writes = 2000
		for depth := range writes {
			done := make(chan error)
			go func() {
				done <- atStackDepth(depth, func() error {
					_, err := conn.Write(request)
					return err
				})
			}()
			require.NoError(t, <-done)
		}

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.Equal(c, before+writes, bucket(), "each write must be parsed exactly once")
		}, 5*time.Second, 100*time.Millisecond)
	})
}

// atStackDepth calls f below depth additional frames.
//
//go:noinline
func atStackDepth(depth int, f func() error) error {
	var frame [64]byte
	if depth == 0 {
		return f()
	}
	frame[depth%len(frame)] = byte(depth)
	err := atStackDepth(depth-1, f)
	runtime.KeepAlive(frame)
	return err
}
