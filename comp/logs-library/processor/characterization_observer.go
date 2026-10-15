// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"bytes"
	"hash/maphash"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/characterization"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const (
	characterizationQueueSize   = 1024
	characterizationSourceLimit = 4096
	characterizationScanLimit   = 4096
)

var (
	datadogSourceKey = []byte("ddsource")
	messageKey       = []byte("message")
	httpMarker       = []byte(` HTTP/`)
)

type characterizationObservation struct {
	contentBytes  int
	rawBytes      int
	tagCount      int
	tagBytes      int
	sourceType    string
	pipeline      string
	payloadFamily string
	hasService    bool
	hasSource     bool
	sourceHash    uint64
	hasSourceID   bool
	observedAt    time.Time
}

type characterizationSourceIdentity struct {
	sourceType string
	pipeline   string
	hash       uint64
}

type characterizationStream struct {
	sourceType string
	pipeline   string
}

type characterizationObserver struct {
	queue        chan characterizationObservation
	done         chan struct{}
	manager      *characterization.Manager
	once         sync.Once
	pipeline     string
	sourceSeed   maphash.Seed
	sourceIDs    map[characterizationSourceIdentity]struct{}
	sourceCounts map[characterizationStream]int
	lastIngress  map[characterizationStream]time.Time
}

func newCharacterizationObserver(pipeline string, manager *characterization.Manager) *characterizationObserver {
	return &characterizationObserver{
		queue:        make(chan characterizationObservation, characterizationQueueSize),
		manager:      manager,
		done:         make(chan struct{}),
		pipeline:     pipeline,
		sourceSeed:   maphash.MakeSeed(),
		sourceIDs:    make(map[characterizationSourceIdentity]struct{}),
		sourceCounts: make(map[characterizationStream]int),
		lastIngress:  make(map[characterizationStream]time.Time),
	}
}

func (o *characterizationObserver) start() {
	metrics.TlmCharacterizationObserverStartTime.Set(float64(time.Now().UnixNano())/float64(time.Second), o.pipeline)
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
	if !o.manager.Active() {
		return
	}
	observation := makeCharacterizationObservation(msg, pipeline, o.sourceSeed)
	o.manager.Record(characterization.MessageObservation{
		ObservedAt:    observation.observedAt,
		ContentBytes:  observation.contentBytes,
		RawBytes:      observation.rawBytes,
		TagCount:      observation.tagCount,
		TagBytes:      observation.tagBytes,
		SourceType:    observation.sourceType,
		Pipeline:      observation.pipeline,
		PayloadFamily: observation.payloadFamily,
		HasService:    observation.hasService,
		HasSource:     observation.hasSource,
		SourceHash:    observation.sourceHash,
		HasSourceID:   observation.hasSourceID,
	})
	select {
	case o.queue <- observation:
	default:
		metrics.TlmCharacterizationObserverDrops.Inc(pipeline)
	}
}

func (o *characterizationObserver) record(observation characterizationObservation) {
	o.recordSource(observation)
	if seconds, ok := o.interarrivalSeconds(observation); ok {
		metrics.TlmCharacterizationInterarrivalSeconds.Observe(seconds, observation.sourceType, observation.pipeline)
	}
	hasService := strconv.FormatBool(observation.hasService)
	hasSource := strconv.FormatBool(observation.hasSource)
	metrics.TlmCharacterizationIngressEvents.Inc(observation.sourceType, observation.pipeline, hasService, hasSource)
	metrics.TlmCharacterizationIngressBytes.Add(float64(observation.contentBytes), observation.sourceType, observation.pipeline)
	metrics.TlmCharacterizationPayloadFamilyEvents.Inc(observation.payloadFamily, observation.pipeline)
	metrics.TlmCharacterizationPayloadFamilyBytes.Add(float64(observation.contentBytes), observation.payloadFamily, observation.pipeline)
	metrics.TlmCharacterizationMessageSizes.Observe(float64(observation.contentBytes), observation.sourceType, observation.pipeline)
	metrics.TlmCharacterizationRawSizes.Observe(float64(observation.rawBytes), observation.sourceType, observation.pipeline)
	metrics.TlmCharacterizationTagCounts.Observe(float64(observation.tagCount), observation.sourceType, observation.pipeline)
	metrics.TlmCharacterizationTagBytes.Observe(float64(observation.tagBytes), observation.sourceType, observation.pipeline)
}

func (o *characterizationObserver) interarrivalSeconds(observation characterizationObservation) (float64, bool) {
	stream := characterizationStream{sourceType: observation.sourceType, pipeline: observation.pipeline}
	previous, found := o.lastIngress[stream]
	if !found {
		o.lastIngress[stream] = observation.observedAt
		return 0, false
	}
	if !observation.observedAt.After(previous) {
		return 0, false
	}
	o.lastIngress[stream] = observation.observedAt
	return observation.observedAt.Sub(previous).Seconds(), true
}

func (o *characterizationObserver) recordSource(observation characterizationObservation) {
	if !observation.hasSourceID {
		metrics.TlmCharacterizationSourceIdentityMissing.Inc(observation.sourceType, observation.pipeline)
		return
	}
	identity := characterizationSourceIdentity{
		sourceType: observation.sourceType,
		pipeline:   observation.pipeline,
		hash:       observation.sourceHash,
	}
	if _, found := o.sourceIDs[identity]; found {
		return
	}
	if len(o.sourceIDs) >= characterizationSourceLimit {
		metrics.TlmCharacterizationSourceCardinalitySaturated.Set(1, observation.sourceType, observation.pipeline)
		return
	}
	o.sourceIDs[identity] = struct{}{}
	stream := characterizationStream{sourceType: observation.sourceType, pipeline: observation.pipeline}
	o.sourceCounts[stream]++
	metrics.TlmCharacterizationSourceCardinality.Set(
		float64(o.sourceCounts[stream]), observation.sourceType, observation.pipeline,
	)
}

func makeCharacterizationObservation(msg *message.Message, pipeline string, sourceSeed maphash.Seed) characterizationObservation {
	observation := characterizationObservation{
		contentBytes:  len(msg.GetContent()),
		rawBytes:      msg.RawDataLen,
		sourceType:    characterizationSourceType(msg),
		pipeline:      pipeline,
		payloadFamily: characterizationPayloadFamily(msg.GetContent()),
		observedAt:    time.Now(),
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

func characterizationPayloadFamily(content []byte) string {
	end := min(len(content), characterizationScanLimit)
	sample := content[:end]
	start := 0
	for start < len(sample) && (sample[start] == ' ' || sample[start] == '\t' || sample[start] == '\r' || sample[start] == '\n') {
		start++
	}
	sample = sample[start:]
	if len(sample) == 0 {
		return "empty"
	}
	if characterizationIsSyslog5424(sample) {
		return "syslog5424"
	}
	switch sample[0] {
	case '{':
		if characterizationHasTopLevelDatadogJSONKeys(sample) {
			return "datadog_json"
		}
		return "json"
	case '[':
		return "json"
	}
	if sample[0] >= '0' && sample[0] <= '9' && bytes.Contains(sample, httpMarker) {
		return "apache_common"
	}
	return "plain"
}

func characterizationHasTopLevelDatadogJSONKeys(sample []byte) bool {
	depth := 0
	hasMessage := false
	hasSource := false
	for index := 0; index < len(sample); index++ {
		switch sample[index] {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case '"':
			end := index + 1
			escaped := false
			for ; end < len(sample); end++ {
				if escaped {
					escaped = false
					continue
				}
				if sample[end] == '\\' {
					escaped = true
					continue
				}
				if sample[end] == '"' {
					break
				}
			}
			if end >= len(sample) {
				return false
			}
			if depth == 1 {
				next := end + 1
				for next < len(sample) && (sample[next] == ' ' || sample[next] == '\t' || sample[next] == '\r' || sample[next] == '\n') {
					next++
				}
				if next < len(sample) && sample[next] == ':' {
					key := sample[index+1 : end]
					hasMessage = hasMessage || bytes.Equal(key, messageKey)
					hasSource = hasSource || bytes.Equal(key, datadogSourceKey)
					if hasMessage && hasSource {
						return true
					}
				}
			}
			index = end
		}
	}
	return false
}

func characterizationIsSyslog5424(sample []byte) bool {
	if len(sample) < 6 || sample[0] != 60 {
		return false
	}
	closing := 1
	for closing < len(sample) && closing <= 3 && sample[closing] >= 48 && sample[closing] <= 57 {
		closing++
	}
	if closing == 1 || closing >= len(sample) || sample[closing] != 62 {
		return false
	}
	priority, err := strconv.Atoi(string(sample[1:closing]))
	if err != nil || priority > 191 {
		return false
	}
	version := closing + 1
	return version+1 < len(sample) &&
		sample[version] >= 49 &&
		sample[version] <= 51 &&
		sample[version+1] == 32
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
