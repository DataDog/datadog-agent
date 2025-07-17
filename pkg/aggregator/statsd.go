// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aggregator

import (
	"context"
	"os"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewStatsdDirect creates a direct interface to the dogstatsd demultiplexer, but exposing the statsd.ClientInterface
func NewStatsdDirect(demux DemultiplexerWithAggregator, hostnameComp hostnameinterface.Component) (ddgostatsd.ClientInterface, error) {
	eventsChan, serviceCheckChan := demux.GetEventsAndServiceChecksChannels()
	hostname, err := hostnameComp.Get(context.TODO())
	if err != nil {
		log.Warnf("error getting hostname for statsd direct client: %s", err)
		hostname = ""
	}
	return &statsdDirect{
		demux:            demux,
		eventsChan:       eventsChan,
		serviceCheckChan: serviceCheckChan,
		origin: taggertypes.OriginInfo{
			LocalData:     origindetection.LocalData{ProcessID: uint32(os.Getpid())},
			ProductOrigin: origindetection.ProductOriginDogStatsD,
		},
		hostname: hostname,
	}, nil
}

var _ ddgostatsd.ClientInterface = (*statsdDirect)(nil)

type statsdDirect struct {
	demux            DemultiplexerWithAggregator
	eventsChan       chan []*event.Event
	serviceCheckChan chan []*servicecheck.ServiceCheck
	origin           taggertypes.OriginInfo
	hostname         string
}

func (s statsdDirect) Gauge(name string, value float64, tags []string, rate float64) error {
	s.demux.AggregateSample(metrics.MetricSample{
		Name:       name,
		Value:      value,
		Mtype:      metrics.GaugeType,
		Tags:       tags,
		Host:       s.hostname,
		SampleRate: rate,
		Timestamp:  float64(time.Now().Unix()),
		OriginInfo: s.origin,
	})
	return nil
}

func (s statsdDirect) GaugeWithTimestamp(name string, value float64, tags []string, rate float64, timestamp time.Time) error {
	s.demux.SendSamplesWithoutAggregation([]metrics.MetricSample{{
		Name:       name,
		Value:      value,
		Mtype:      metrics.GaugeType,
		Tags:       tags,
		Host:       s.hostname,
		SampleRate: rate,
		Timestamp:  float64(timestamp.Unix()),
		OriginInfo: s.origin,
	}})
	return nil
}

func (s statsdDirect) Count(name string, value int64, tags []string, rate float64) error {
	s.demux.AggregateSample(metrics.MetricSample{
		Name:       name,
		Value:      float64(value),
		Mtype:      metrics.CountType,
		Tags:       tags,
		Host:       s.hostname,
		SampleRate: rate,
		Timestamp:  float64(time.Now().Unix()),
		OriginInfo: s.origin,
	})
	return nil
}

func (s statsdDirect) CountWithTimestamp(name string, value int64, tags []string, rate float64, timestamp time.Time) error {
	s.demux.SendSamplesWithoutAggregation([]metrics.MetricSample{{
		Name:       name,
		Value:      float64(value),
		Mtype:      metrics.CountType,
		Tags:       tags,
		Host:       s.hostname,
		SampleRate: rate,
		Timestamp:  float64(timestamp.Unix()),
		OriginInfo: s.origin,
	}})
	return nil
}

func (s statsdDirect) Histogram(name string, value float64, tags []string, rate float64) error {
	s.demux.AggregateSample(metrics.MetricSample{
		Name:       name,
		Value:      value,
		Mtype:      metrics.HistogramType,
		Tags:       tags,
		Host:       s.hostname,
		SampleRate: rate,
		Timestamp:  float64(time.Now().Unix()),
		OriginInfo: s.origin,
	})
	return nil
}

func (s statsdDirect) Distribution(name string, value float64, tags []string, rate float64) error {
	s.demux.AggregateSample(metrics.MetricSample{
		Name:       name,
		Value:      value,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		Host:       s.hostname,
		SampleRate: rate,
		Timestamp:  float64(time.Now().Unix()),
		OriginInfo: s.origin,
	})
	return nil
}

func (s statsdDirect) Decr(name string, tags []string, rate float64) error {
	return s.Count(name, -1, tags, rate)
}

func (s statsdDirect) Incr(name string, tags []string, rate float64) error {
	return s.Count(name, 1, tags, rate)
}

func (s statsdDirect) Set(name string, value string, tags []string, rate float64) error {
	s.demux.AggregateSample(metrics.MetricSample{
		Name:       name,
		RawValue:   value,
		Mtype:      metrics.SetType,
		Tags:       tags,
		Host:       s.hostname,
		SampleRate: rate,
		Timestamp:  float64(time.Now().Unix()),
		OriginInfo: s.origin,
	})
	return nil
}

func (s statsdDirect) Timing(name string, value time.Duration, tags []string, rate float64) error {
	return s.TimeInMilliseconds(name, float64(value.Milliseconds()), tags, rate)
}

func (s statsdDirect) TimeInMilliseconds(name string, value float64, tags []string, rate float64) error {
	s.demux.AggregateSample(metrics.MetricSample{
		Name:       name,
		Value:      value,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		Host:       s.hostname,
		SampleRate: rate,
		Timestamp:  float64(time.Now().Unix()),
		OriginInfo: s.origin,
	})
	return nil
}

func (s statsdDirect) Event(e *ddgostatsd.Event) error {
	p, _ := event.GetEventPriorityFromString(string(e.Priority))
	at, _ := event.GetAlertTypeFromString(string(e.AlertType))
	s.eventsChan <- []*event.Event{{
		Title:          e.Title,
		Text:           e.Text,
		Ts:             e.Timestamp.Unix(),
		Priority:       p,
		Host:           e.Hostname,
		Tags:           e.Tags,
		AlertType:      at,
		AggregationKey: e.AggregationKey,
		SourceTypeName: e.SourceTypeName,
		EventType:      "",
		OriginInfo:     s.origin,
	}}
	return nil
}

func (s statsdDirect) SimpleEvent(title, text string) error {
	s.eventsChan <- []*event.Event{{
		Title:      title,
		Text:       text,
		OriginInfo: s.origin,
	}}
	return nil
}

func (s statsdDirect) ServiceCheck(sc *ddgostatsd.ServiceCheck) error {
	s.serviceCheckChan <- []*servicecheck.ServiceCheck{{
		CheckName:  sc.Name,
		Host:       sc.Hostname,
		Ts:         sc.Timestamp.Unix(),
		Status:     servicecheck.ServiceCheckStatus(sc.Status),
		Message:    sc.Message,
		Tags:       sc.Tags,
		OriginInfo: s.origin,
	}}
	return nil
}

func (s statsdDirect) SimpleServiceCheck(name string, status ddgostatsd.ServiceCheckStatus) error {
	s.serviceCheckChan <- []*servicecheck.ServiceCheck{{
		CheckName:  name,
		Status:     servicecheck.ServiceCheckStatus(status),
		OriginInfo: s.origin,
	}}
	return nil
}

func (s statsdDirect) Close() error {
	return nil
}

func (s statsdDirect) Flush() error {
	return nil
}

func (s statsdDirect) IsClosed() bool {
	return false
}

func (s statsdDirect) GetTelemetry() ddgostatsd.Telemetry {
	return ddgostatsd.Telemetry{}
}
