// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const characterizationQueueSize = 1024

type characterizationObservation struct {
	contentBytes int
	rawBytes     int
	tagCount     int
	tagBytes     int
	sourceType   string
	pipeline     string
	hasService   bool
	hasSource    bool
}

type characterizationObserver struct {
	queue chan characterizationObservation
	done  chan struct{}
	once  sync.Once
}

func newCharacterizationObserver() *characterizationObserver {
	return &characterizationObserver{
		queue: make(chan characterizationObservation, characterizationQueueSize),
		done:  make(chan struct{}),
	}
}

func (o *characterizationObserver) start() {
	go func() {
		defer close(o.done)
		for observation := range o.queue {
			o.record(observation)
		}
	}()
}

func (o *characterizationObserver) stop() {
	o.once.Do(func() { close(o.queue) })
	<-o.done
}

func (o *characterizationObserver) observe(msg *message.Message, pipeline string) {
	observation := makeCharacterizationObservation(msg, pipeline)
	select {
	case o.queue <- observation:
	default:
		metrics.TlmCharacterizationObserverDrops.Inc(pipeline)
	}
}

func (o *characterizationObserver) record(observation characterizationObservation) {
	hasService := strconv.FormatBool(observation.hasService)
	hasSource := strconv.FormatBool(observation.hasSource)
	metrics.TlmCharacterizationIngressEvents.Inc(observation.sourceType, observation.pipeline, hasService, hasSource)
	metrics.TlmCharacterizationIngressBytes.Add(float64(observation.contentBytes), observation.sourceType, observation.pipeline)
	metrics.TlmCharacterizationMessageSizes.Observe(float64(observation.contentBytes), observation.sourceType, observation.pipeline)
	metrics.TlmCharacterizationRawSizes.Observe(float64(observation.rawBytes), observation.sourceType, observation.pipeline)
	metrics.TlmCharacterizationTagCounts.Observe(float64(observation.tagCount), observation.sourceType, observation.pipeline)
	metrics.TlmCharacterizationTagBytes.Observe(float64(observation.tagBytes), observation.sourceType, observation.pipeline)
}

func makeCharacterizationObservation(msg *message.Message, pipeline string) characterizationObservation {
	observation := characterizationObservation{
		contentBytes: len(msg.GetContent()),
		rawBytes:     msg.RawDataLen,
		sourceType:   characterizationSourceType(msg),
		pipeline:     pipeline,
	}
	if msg.Origin != nil {
		observation.tagCount, observation.tagBytes = msg.Origin.TagMetadataStats(msg.ParsingExtra.Tags)
		observation.hasService = msg.Origin.Service() != ""
		observation.hasSource = msg.Origin.Source() != ""
	}
	return observation
}

func characterizationSourceType(msg *message.Message) string {
	if msg.Origin == nil || msg.Origin.LogSource == nil || msg.Origin.LogSource.Config == nil {
		return "unknown"
	}
	sourceType := msg.Origin.LogSource.Config.Type
	switch sourceType {
	case logsconfig.TCPType,
		logsconfig.UDPType,
		logsconfig.FileType,
		logsconfig.DockerType,
		logsconfig.ContainerdType,
		logsconfig.JournaldType,
		logsconfig.IntegrationType,
		logsconfig.WindowsEventType,
		logsconfig.StringChannelType:
		return sourceType
	default:
		return "unknown"
	}
}
