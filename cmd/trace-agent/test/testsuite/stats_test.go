// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
	"github.com/DataDog/datadog-agent/cmd/trace-agent/test/testsuite/testdata"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	protoutil "github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/runtime/protoiface"
)

func TestClientStats(t *testing.T) {
	var r test.Runner
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()

	for _, tt := range testdata.ClientStatsTests {
		t.Run("", func(t *testing.T) {
			if err := r.RunAgent([]byte("hostname: agent-hostname\r\napm_config:\r\n  env: agent-env")); err != nil {
				t.Fatal(err)
			}
			defer r.KillAgent()

			if err := r.PostMsgpack("/v0.6/stats", tt.In); err != nil {
				t.Fatal(err)
			}
			timeout := time.After(3 * time.Second)
			out := r.Out()
			res := make([]*pb.StatsPayload, 0, len(tt.Out))
			for {
				select {
				case p := <-out:
					got, ok := p.(*pb.StatsPayload)
					if !ok {
						continue
					}
					got = normalizeTimeFields(t, got)
					res = append(res, got)
					if len(res) < len(tt.Out) {
						continue
					}
					assert.Equalf(t, len(res), len(tt.Out), "res had so many elements: %d\ntt has:%d", len(res), len(tt.Out))
					actual := []protoiface.MessageV1{}
					expected := []protoiface.MessageV1{}
					for _, msg := range res {
						actual = append(actual, msg)
					}
					for _, msg := range tt.Out {
						expected = append(expected, msg)
					}
					assert.ElementsMatch(t, protoutil.ToStringSlice(actual), protoutil.ToStringSlice(expected))
					return
				case <-timeout:
					t.Fatalf("timed out, log was:\n%s", r.AgentLog())
				}
			}
		})
	}
}

func normalizeTimeFields(t *testing.T, p *pb.StatsPayload) *pb.StatsPayload {
	now := time.Now().UnixNano()
	for _, s := range p.Stats {
		for i := range s.Stats {
			assert.True(t, s.Stats[i].AgentTimeShift > now-100*1e9)
			s.Stats[i].AgentTimeShift = 0
			assert.True(t, s.Stats[i].Start >= uint64(now-40*1e9))
			s.Stats[i].Start = 0
		}
	}
	return p
}
