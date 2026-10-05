// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"hash/maphash"
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const (
	characterizationQueueSize   = 1024
	characterizationSourceLimit = 4096
)

type characterizationObservation struct {
	contentBytes int
	rawBytes     int
	tagCount     int
	tagBytes     int
	sourceType   string
	pipeline     string
	hasService   bool
	hasSource    bool
	sourceHash   uint64
	hasSourceID  bool
}

type characterizationSourceIdentity struct {
	sourceType string
	hash       uint64
}

type characterizationObserver struct {
	queue        chan characterizationObservation
	done         chan struct{}
	once         sync.Once
	sourceSeed   maphash.Seed
	sourceIDs    map[characterizationSourceIdentity]struct{}
	sourceCounts map[string]int
}

func newCharacterizationObserver() *characterizationObserver {
	return &characterizationObserver{
		queue:        make(chan characterizationObservation, characterizationQueueSize),
		done:         make(chan struct{}),
		sourceSeed:   maphash.MakeSeed(),
		sourceIDs:    make(map[characterizationSourceIdentity]struct{}),
		sourceCounts: make(map[string]int),
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
	observation := makeCharacterizationObservation(msg, pipeline, o.sourceSeed)
	select {
	case o.queue <- observation:
	default:
		metrics.TlmCharacterizationObserverDrops.Inc(pipeline)
	}
}

func (o *characterizationObserver) record(observation characterizationObservation) {
	o.recordSource(observation)
	hasService := strconv.FormatBool(observation.hasService)
	hasSource := strconv.FormatBool(observation.hasSource)
	metrics.TlmCharacterizationIngressEvents.Inc(observation.sourceType, observation.pipeline, hasService, hasSource)
	metrics.TlmCharacterizationIngressBytes.Add(float64(observation.contentBytes), observation.sourceType, observation.pipeline)
	metrics.TlmCharacterizationMessageSizes.Observe(float64(observation.contentBytes), observation.sourceType, observation.pipeline)
	metrics.TlmCharacterizationRawSizes.Observe(float64(observation.rawBytes), observation.sourceType, observation.pipeline)
	metrics.TlmCharacterizationTagCounts.Observe(float64(observation.tagCount), observation.sourceType, observation.pipeline)
	metrics.TlmCharacterizationTagBytes.Observe(float64(observation.tagBytes), observation.sourceType, observation.pipeline)
}

func (o *characterizationObserver) recordSource(observation characterizationObservation) {
	if !observation.hasSourceID {
		metrics.TlmCharacterizationSourceIdentityMissing.Inc(observation.sourceType, observation.pipeline)
		return
	}
	identity := characterizationSourceIdentity{sourceType: observation.sourceType, hash: observation.sourceHash}
	if _, found := o.sourceIDs[identity]; found {
		return
	}
	if len(o.sourceIDs) >= characterizationSourceLimit {
		metrics.TlmCharacterizationSourceCardinalitySaturated.Set(1, observation.sourceType, observation.pipeline)
		return
	}
	o.sourceIDs[identity] = struct{}{}
	o.sourceCounts[observation.sourceType]++
	metrics.TlmCharacterizationSourceCardinality.Set(
		float64(o.sourceCounts[observation.sourceType]), observation.sourceType, observation.pipeline,
	)
}

func makeCharacterizationObservation(msg *message.Message, pipeline string, sourceSeed maphash.Seed) characterizationObservation {
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
	observation.sourceHash, observation.hasSourceID = characterizationSourceHash(msg, sourceSeed)
	return observation
}

func characterizationSourceHash(msg *message.Message, seed maphash.Seed) (uint64, bool) {
	if msg.Origin == nil {
		return 0, false
	}
	identifier := msg.Origin.Identifier
	if identifier == "" {
		identifier = msg.Origin.FilePath
	}
	if identifier == "" {
		return 0, false
	}
	return maphash.String(seed, identifier), true
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
