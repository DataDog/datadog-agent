// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package testsuite

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
	"github.com/DataDog/datadog-agent/cmd/trace-agent/test/testsuite/testdata"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

func TestProxy(t *testing.T) {
	var r test.Runner
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()
	var proxyt, proxys bool
	director := func(req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/traces") {
			proxyt = true
		}
		if strings.HasSuffix(req.URL.Path, "/stats") {
			proxys = true
		}
	}
	proxy := httptest.NewServer(&httputil.ReverseProxy{Director: director})

	if err := r.RunAgent([]byte(fmt.Sprintf(`
hostname: agent-hostname
apm_config:
  env: agent-env

proxy:
  http: %[1]s
  https: %[1]s
`, proxy.URL))); err != nil {
		t.Fatal(err)
	}

	p := testutil.GeneratePayload(10, &testutil.TraceConfig{
		MinSpans: 10,
		Keep:     true,
	}, nil)
	if err := r.Post(p); err != nil {
		t.Fatal(err)
	}
	if err := r.PostMsgpack("/v0.6/stats", testdata.ClientStatsTests[0].In); err != nil {
		t.Fatal(err)
	}
	defer r.KillAgent()
	timeout := time.After(3 * time.Second)
	out := r.Out()
	var gott, gots bool
	for {
		select {
		case p := <-out:
			switch p.(type) {
			case *pb.StatsPayload:
				gots = true
			case *pb.AgentPayload:
				gott = true
			}
			if gott && gots {
				// got traces and got stats
				if !proxyt {
					t.Fatal("Did not proxy through for /traces")
				}
				if !proxys {
					t.Fatal("Did not proxy through for /stats")
				}
				return
			}
		case <-timeout:
			t.Fatalf("timed out waiting for payloads, log was:\n%s", r.AgentLog())
		}
	}
}
