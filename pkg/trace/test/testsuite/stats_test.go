package testsuite

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
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

	os.Setenv("DD_APM_FEATURES", "client_stats")
	for _, tt := range testdata.ClientStatsTests {
		t.Run("", func(t *testing.T) {
			if err := r.RunAgent(nil); err != nil {
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
					got, ok := p.(stats.Payload)
					if !ok {
						continue
					}
					if reflect.DeepEqual(got, tt.Out) {
						return
					}
					t.Logf("%#v", got)
					t.Fatal("did not match")
				case <-timeout:
					t.Fatalf("timed out, log was:\n%s", r.AgentLog())
				}
			}
		})
	}

	os.Unsetenv("DD_APM_FEATURES")
	t.Run("off", func(t *testing.T) {
		if err := r.RunAgent(nil); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		err := r.PostMsgpack("/v0.5/stats", &pb.ClientStatsPayload{})
		if err == nil {
			t.Fatal()
		}
		if !strings.Contains(err.Error(), "404") {
			t.Fatal()
		}
	})
}
