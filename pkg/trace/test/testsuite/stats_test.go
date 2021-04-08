package testsuite

import (
	"reflect"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/test"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testsuite/testdata"
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

			if err := r.PostMsgpack("/v0.5/stats", &tt.In); err != nil {
				t.Fatal(err)
			}
			timeout := time.After(3 * time.Second)
			out := r.Out()
			for {
				select {
				case p := <-out:
					got, ok := p.(pb.StatsPayload)
					if !ok {
						continue
					}
					if reflect.DeepEqual(got, tt.Out) {
						return
					}
					t.Logf("got: %#v", got)
					t.Logf("expected: %#v", tt.Out)
					t.Fatal("did not match")
				case <-timeout:
					t.Fatalf("timed out, log was:\n%s", r.AgentLog())
				}
			}
		})
	}
}
