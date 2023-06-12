// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"context"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestOTLPIngest(t *testing.T) {
	var r test.Runner
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()

	t.Run("passthrough", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		c := fmt.Sprintf(`
otlp_config:
  traces:
    internal_port: %d
  receiver:
    grpc:
      endpoint: 0.0.0.0:5111
apm_config:
  env: my-env
`, port)
		if err := r.RunAgent([]byte(c)); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatal("Error dialing: ", err)
		}
		client := ptraceotlp.NewGRPCClient(conn)
		now := uint64(time.Now().UnixNano())
		pack := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
			{
				LibName:    "test",
				LibVersion: "0.1t",
				Attributes: map[string]interface{}{"service.name": "pylons"},
				Spans: []*testutil.OTLPSpan{
					{
						Name:       "/path",
						Kind:       ptrace.SpanKindServer,
						Start:      now,
						End:        now + 200000000,
						Attributes: map[string]interface{}{"name": "john"},
					},
				},
			},
		})
		_, err = client.Export(context.Background(), pack)
		if err != nil {
			log.Fatal("Error calling: ", err)
		}
		waitForTrace(t, &r, func(p *pb.AgentPayload) {
			assert := assert.New(t)
			assert.Equal(p.Env, "my-env")
			assert.Len(p.TracerPayloads, 1)
			assert.Len(p.TracerPayloads[0].Chunks, 1)
			assert.Len(p.TracerPayloads[0].Chunks[0].Spans, 1)
			assert.Equal(p.TracerPayloads[0].Chunks[0].Spans[0].Meta["name"], "john")
		})
	})

	// regression test for DataDog/datadog-agent#11297
	t.Run("duplicate-spanID", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		c := fmt.Sprintf(`
otlp_config:
  traces:
    internal_port: %d
  receiver:
    grpc:
      endpoint: 0.0.0.0:5111
apm_config:
  env: my-env
`, port)
		if err := r.RunAgent([]byte(c)); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatal("Error dialing: ", err)
		}
		client := ptraceotlp.NewGRPCClient(conn)
		now := uint64(time.Now().UnixNano())
		pack := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
			{
				LibName:    "test",
				LibVersion: "0.1t",
				Attributes: map[string]interface{}{"service.name": "pylons"},
				Spans: []*testutil.OTLPSpan{
					{
						TraceID: testutil.OTLPFixedTraceID,
						SpanID:  testutil.OTLPFixedSpanID,
						Name:    "/path",
						Kind:    ptrace.SpanKindServer,
						Start:   now,
						End:     now + 200000000,
					},
					{
						TraceID: testutil.OTLPFixedTraceID,
						SpanID:  testutil.OTLPFixedSpanID,
						Name:    "/path",
						Kind:    ptrace.SpanKindServer,
						Start:   now,
						End:     now + 200000000,
					},
				},
			},
		})
		_, err = client.Export(context.Background(), pack)
		if err != nil {
			log.Fatal("Error calling: ", err)
		}
		timeout := time.After(1 * time.Second)
	loop:
		for {
			select {
			case <-timeout:
				t.Fatal("Timed out waiting for duplicate SpanID warning.")
			default:
				time.Sleep(10 * time.Millisecond)
				if strings.Contains(r.AgentLog(), `Found malformed trace with duplicate span ID (reason:duplicate_span_id): service:"pylons"`) {
					break loop
				}
			}
		}
	})
}
