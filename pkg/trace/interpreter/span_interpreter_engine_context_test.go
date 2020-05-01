package interpreter

import (
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/interpreter/model"
	"github.com/StackVista/stackstate-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSpanInterpreterEngineContext(t *testing.T) {
	siec := MakeSpanInterpreterEngineContext(config.DefaultInterpreterConfig())

	// assert nano to millis conversion
	actualMillis := siec.nanosToMillis(1581423873198300479)
	expectedMillis := int64(1581423873198)

	assert.Equal(t, actualMillis, expectedMillis)

	// assert extract span metadata
	actualMeta, err := siec.extractSpanMetadata(&pb.Span{
		Type: "some-type",
		Meta: map[string]string{
			"span.starttime": "1586441095", //Thursday, 9 April 2020 14:04:55
			"span.hostname":  "hostname",
			"span.pid":       "10",
			"span.kind":      "some-kind",
		},
	})
	expectedMeta := model.SpanMetadata{
		CreateTime: int64(1586441095),
		Hostname:   "hostname",
		PID:        int(10),
		Type:       "some-type",
		Kind:       "some-kind",
	}

	assert.Equal(t, err, nil)
	assert.EqualValues(t, expectedMeta, *actualMeta)

	// assert extract span metadata, default to Start is Meta span.starttime is missing
	actualMeta, err = siec.extractSpanMetadata(&pb.Span{
		Start: int64(1586441095000000), //Thursday, 9 April 2020 14:04:55 in nano seconds
		Type:  "some-type",
		Meta: map[string]string{
			"span.hostname": "hostname",
			"span.pid":      "10",
			"span.kind":     "some-kind",
		},
	})
	expectedMeta = model.SpanMetadata{
		CreateTime: int64(1586441095),
		Hostname:   "hostname",
		PID:        int(10),
		Type:       "some-type",
		Kind:       "some-kind",
	}

	assert.Equal(t, err, nil)
	assert.EqualValues(t, expectedMeta, *actualMeta)
}
