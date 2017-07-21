package percentile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshalJSONSketchSeries(t *testing.T) {

	payload := []byte("{\"sketch_series\":[{\"metric\":\"test.metrics\",\"tags\":[\"tag:yes\"],\"host\":\"localHost\",\"interval\":0,\"sketches\":[{\"timestamp\":12345,\"qsketch\":{\"entries\":[[1,1,0]],\"min\":1,\"max\":1,\"n\":1,\"sum\":1,\"avg\":1}}]}]}\n")

	sketch := QSketch{
		GKArray{Entries: []Entry{{1, 1, 0}},
			Min: 1, Count: 1, Sum: 1, Avg: 1, Max: 1},
	}

	data, err := UnmarshalJSONSketchSeries(payload)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(data))

	assert.Equal(t, "test.metrics", data[0].Name)
	assert.Equal(t, "tag:yes", data[0].Tags[0])
	assert.Equal(t, "localHost", data[0].Host)
	assert.Equal(t, int64(0), data[0].Interval)
	assert.Equal(t, Sketch{Timestamp: 12345, Sketch: sketch}, data[0].Sketches[0])
}
