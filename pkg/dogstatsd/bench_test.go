package dogstatsd

import (
	"testing"
)

// TODO: remove before merging

func BenchmarkCKeyLookup(b *testing.B) {
	for n := 0; n < b.N; n++ {
		_, _ = parseMetricMessage([]byte("daemon:666|g|#sometag1:somevalue1,host:my-hostname,sometag2:somevalue2"))
		_, _ = parseServiceCheckMessage([]byte("_sc|agent.up|0|d:21|h:localhost|#tag1:test,tag2|m:this is fine"))
		_, _ = parseEventMessage([]byte("_e{10,9}:test title|test text|k:some aggregation key"))
	}
}
