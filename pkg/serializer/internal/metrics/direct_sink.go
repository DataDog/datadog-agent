// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	pkgmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// DirectSeriesSink serializes series directly from the producer goroutine into
// the configured pipeline builders. It is an intentionally experimental sink
// used to bypass IterableSeries/channel traversal in local DogStatsD
// aggregator/serializer experiments.
type DirectSeriesSink struct {
	pbs           []serieWriter
	segmentShadow *segmentShadowBuilder
	start         time.Time
	count         uint64
	rowCount      uint64
	err           error
}

// NewDirectSeriesSink creates a direct series sink for the given pipeline set.
func NewDirectSeriesSink(config config.Component, strategy compression.Component, pipelines PipelineSet) (*DirectSeriesSink, error) {
	sink := &DirectSeriesSink{
		pbs:           make([]serieWriter, 0, len(pipelines)),
		segmentShadow: newSegmentShadowBuilder(),
		start:         time.Now(),
	}

	for pipelineConfig, pipelineContext := range pipelines {
		var sw serieWriter
		if !pipelineConfig.V3 {
			bufferContext := marshaler.NewBufferContext()
			pb, err := (&IterableSeries{}).NewPayloadsBuilder(bufferContext, config, strategy, pipelineConfig, pipelineContext)
			if err != nil {
				return nil, err
			}
			sw = &pb
		} else {
			pb, err := newPayloadsBuilderV3WithConfig(config, strategy, pipelineConfig, pipelineContext)
			if err != nil {
				return nil, err
			}
			sw = pb
		}

		if err := sw.startPayload(); err != nil {
			return nil, err
		}
		sink.pbs = append(sink.pbs, sw)
	}

	return sink, nil
}

// Append serializes a serie into each configured pipeline.
func (sink *DirectSeriesSink) Append(serie *pkgmetrics.Serie) {
	if sink.err != nil {
		return
	}
	if serie == nil {
		return
	}

	sink.count++
	for _, pb := range sink.pbs {
		if err := pb.writeSerie(serie); err != nil {
			sink.err = err
			return
		}
	}
	sink.segmentShadow.observeSerie(serie)
}

// AppendSerieRow serializes a producer-side row into each configured pipeline.
func (sink *DirectSeriesSink) AppendSerieRow(row pkgmetrics.SerieRow) {
	if sink.err != nil {
		return
	}

	row.NormalizeSpecialTags()
	sink.count++
	sink.rowCount++
	for _, pb := range sink.pbs {
		if rowWriter, ok := pb.(serieRowWriter); ok {
			if err := rowWriter.writeSerieRow(&row); err != nil {
				sink.err = err
				return
			}
			continue
		}

		if err := pb.writeSerie(row.ToSerie()); err != nil {
			sink.err = err
			return
		}
	}
	sink.segmentShadow.observeSerieRow(&row)
}

// AppendV3MetricPointRow serializes a single-point producer-side v3 row into
// each configured pipeline without constructing a SerieRow on the v3 path.
func (sink *DirectSeriesSink) AppendV3MetricPointRow(row pkgmetrics.V3MetricPointRow) {
	if sink.err != nil {
		return
	}

	row.NormalizeSpecialTags()
	sink.count++
	sink.rowCount++
	for _, pb := range sink.pbs {
		if pointWriter, ok := pb.(v3MetricPointRowWriter); ok {
			if err := pointWriter.writeV3MetricPointRow(&row); err != nil {
				sink.err = err
				return
			}
			continue
		}

		serieRow := row.ToSerieRow()
		if rowWriter, ok := pb.(serieRowWriter); ok {
			if err := rowWriter.writeSerieRow(&serieRow); err != nil {
				sink.err = err
				return
			}
			continue
		}

		if err := pb.writeSerie(serieRow.ToSerie()); err != nil {
			sink.err = err
			return
		}
	}
	sink.segmentShadow.observeV3MetricPointRow(&row)
}

// Finish finalizes all active payload builders and returns the number of
// producer-visible series appended to the sink.
func (sink *DirectSeriesSink) Finish() (uint64, error) {
	phase := "direct_series"
	if sink.rowCount > 0 {
		phase = "direct_series_rows"
	}
	sink.segmentShadow.finish(phase, time.Since(sink.start))
	err := sink.err
	for i := range sink.pbs {
		if finishErr := sink.pbs[i].finishPayload(); finishErr != nil && err == nil {
			err = finishErr
		}
	}
	recordSeriesPipelineDuration(phase, time.Since(sink.start))
	return sink.count, err
}

// DirectSketchSink serializes sketch series directly from the producer
// goroutine into the configured pipeline builders.
type DirectSketchSink struct {
	pbs           []sketchWriter
	segmentShadow *segmentShadowBuilder
	start         time.Time
	count         uint64
	err           error
}

// NewDirectSketchSink creates a direct sketch sink for the given pipeline set.
func NewDirectSketchSink(config config.Component, strategy compression.Component, pipelines PipelineSet, logger log.Component) (*DirectSketchSink, error) {
	sink := &DirectSketchSink{
		pbs:           make([]sketchWriter, 0, len(pipelines)),
		segmentShadow: newSegmentShadowBuilder(),
		start:         time.Now(),
	}

	for pipelineConfig, pipelineContext := range pipelines {
		var sw sketchWriter
		if !pipelineConfig.V3 {
			bufferContext := marshaler.NewBufferContext()
			sw = newPayloadsBuilder(bufferContext, config, strategy, logger, pipelineConfig, pipelineContext)
		} else {
			pb, err := newPayloadsBuilderV3WithConfig(config, strategy, pipelineConfig, pipelineContext)
			if err != nil {
				return nil, err
			}
			sw = pb
		}

		if err := sw.startPayload(); err != nil {
			return nil, err
		}
		sink.pbs = append(sink.pbs, sw)
	}

	return sink, nil
}

// Append serializes a sketch series into each configured pipeline.
func (sink *DirectSketchSink) Append(sketch *pkgmetrics.SketchSeries) {
	if sink.err != nil {
		return
	}
	if sketch == nil {
		return
	}

	sink.count++
	for _, pb := range sink.pbs {
		if err := pb.writeSketch(sketch); err != nil {
			sink.err = err
			return
		}
	}
	sink.segmentShadow.observeSketch(sketch)
}

// Finish finalizes all active payload builders and returns the number of
// producer-visible sketch series appended to the sink.
func (sink *DirectSketchSink) Finish() (uint64, error) {
	sink.segmentShadow.finish("direct_sketches", time.Since(sink.start))
	if sink.count == 0 {
		recordSeriesPipelineDuration("direct_sketches", time.Since(sink.start))
		return 0, sink.err
	}

	err := sink.err
	for i := range sink.pbs {
		if finishErr := sink.pbs[i].finishPayload(); finishErr != nil && err == nil {
			err = finishErr
		}
	}
	recordSeriesPipelineDuration("direct_sketches", time.Since(sink.start))
	return sink.count, err
}
