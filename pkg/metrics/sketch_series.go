package metrics

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/quantile"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// A SketchSeries is a timeseries of quantile sketches.
type SketchSeries struct {
	Name       string
	Tags       []string
	Host       string
	Interval   int64
	Points     []SketchPoint
	ContextKey ckey.ContextKey
}

// A SketchPoint represents a quantile sketch at a specific time
type SketchPoint struct {
	Sketch *quantile.Sketch
	Ts     int64
}

type SketchSeriesList []SketchSeries

func (ssl SketchSeriesList) MarshalJSON() ([]byte, error) {
	return nil, errors.New("sketches don't support json encoding")
}

func (ssl SketchSeriesList) Marshal() ([]byte, error) {
	panic("todo")
}

func (ssl SketchSeriesList) SplitPayload(int) ([]marshaler.Marshaler, error) {
	panic("todo")
}
