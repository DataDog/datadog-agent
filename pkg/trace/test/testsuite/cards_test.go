package testsuite

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/test"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/stretchr/testify/assert"
)

func TestCreditCards(t *testing.T) {
	var r test.Runner
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()

	for _, tt := range []struct {
		conf []byte
		out  string
	}{
		{
			conf: []byte(""),
			out:  "4166 6766 6766 6746",
		},
		{
			conf: []byte(`
apm_config:
  env: my-env
  obfuscation:
    credit_cards:
      enabled: true`),
			out: "?",
		},
	} {
		t.Run(tt.out, func(t *testing.T) {
			if err := r.RunAgent(tt.conf); err != nil {
				t.Fatal(err)
			}
			defer r.KillAgent()

			p := testutil.GeneratePayload(1, &testutil.TraceConfig{
				MaxSpans: 1,
				Keep:     true,
			}, &testutil.SpanConfig{MinTags: 2})
			p[0][0].Meta["credit_card_number"] = "4166 6766 6766 6746"
			if err := r.Post(p); err != nil {
				t.Fatal(err)
			}
			waitForTrace(t, &r, func(v pb.TracePayload) {
				payloadsEqual(t, p, v)
				assert.Equal(t, v.Traces[0].Spans[0].Meta["credit_card_number"], tt.out)
			})
		})
	}
}
