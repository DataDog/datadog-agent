// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package serializer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer/jsonstream"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/split"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	protobufContentType      = "application/x-protobuf"
	jsonContentType          = "application/json"
	payloadVersionHTTPHeader = "DD-Agent-Payload"
	apiKeyReplacement        = "\"apiKey\":\"*************************$1"
)

var (
	// AgentPayloadVersion is the versions of the agent-payload repository
	// used to serialize to protobuf
	AgentPayloadVersion string

	jsonExtraHeaders                    http.Header
	protobufExtraHeaders                http.Header
	jsonExtraHeadersWithCompression     http.Header
	protobufExtraHeadersWithCompression http.Header
)

var apiKeyRegExp = regexp.MustCompile("\"apiKey\":\"*\\w+(\\w{5})")

func init() {
	initExtraHeaders()
}

// initExtraHeaders initializes the global extraHeaders variables.
// Not part of the `init` function body to ease testing
func initExtraHeaders() {
	jsonExtraHeaders = make(http.Header)
	jsonExtraHeaders.Set("Content-Type", jsonContentType)

	jsonExtraHeadersWithCompression = make(http.Header)
	for k := range jsonExtraHeaders {
		jsonExtraHeadersWithCompression.Set(k, jsonExtraHeaders.Get(k))
	}

	protobufExtraHeaders = make(http.Header)
	protobufExtraHeaders.Set("Content-Type", protobufContentType)
	protobufExtraHeaders.Set(payloadVersionHTTPHeader, AgentPayloadVersion)

	protobufExtraHeadersWithCompression = make(http.Header)
	for k := range protobufExtraHeaders {
		protobufExtraHeadersWithCompression.Set(k, protobufExtraHeaders.Get(k))
	}

	if compression.ContentEncoding != "" {
		jsonExtraHeadersWithCompression.Set("Content-Encoding", compression.ContentEncoding)
		protobufExtraHeadersWithCompression.Set("Content-Encoding", compression.ContentEncoding)
	}
}

// MetricSerializer represents the interface of method needed by the aggregator to serialize its data
type MetricSerializer interface {
	SendEvents(e marshaler.StreamJSONMarshalerFactory) error
	SendServiceChecks(sc marshaler.StreamJSONMarshaler) error
	SendSeries(series marshaler.StreamJSONMarshaler) error
	SendSketch(sketches marshaler.Marshaler) error
	SendMetadata(m marshaler.Marshaler) error
	SendJSONToV1Intake(data interface{}) error
}

// Serializer serializes metrics to the correct format and routes the payloads to the correct endpoint in the Forwarder
type Serializer struct {
	Forwarder forwarder.Forwarder

	seriesPayloadBuilder *jsonstream.PayloadBuilder

	// Those variables allow users to blacklist any kind of payload
	// from being sent by the agent. This was introduced for
	// environment where, for example, events or serviceChecks
	// might collect data considered too sensitive (database IP and
	// such). By default every kind of payload is enabled since
	// almost every user won't fall into this use case.
	enableEvents                  bool
	enableSeries                  bool
	enableServiceChecks           bool
	enableSketches                bool
	enableJSONToV1Intake          bool
	enableJSONStream              bool
	enableServiceChecksJSONStream bool
	enableEventsJSONStream        bool
}

// NewSerializer returns a new Serializer initialized
func NewSerializer(forwarder forwarder.Forwarder) *Serializer {
	s := &Serializer{
		Forwarder:                     forwarder,
		seriesPayloadBuilder:          jsonstream.NewPayloadBuilder(),
		enableEvents:                  config.Datadog.GetBool("enable_payloads.events"),
		enableSeries:                  config.Datadog.GetBool("enable_payloads.series"),
		enableServiceChecks:           config.Datadog.GetBool("enable_payloads.service_checks"),
		enableSketches:                config.Datadog.GetBool("enable_payloads.sketches"),
		enableJSONToV1Intake:          config.Datadog.GetBool("enable_payloads.json_to_v1_intake"),
		enableJSONStream:              jsonstream.Available && config.Datadog.GetBool("enable_stream_payload_serialization"),
		enableServiceChecksJSONStream: jsonstream.Available && config.Datadog.GetBool("enable_service_checks_stream_payload_serialization"),
		enableEventsJSONStream:        jsonstream.Available && config.Datadog.GetBool("enable_events_stream_payload_serialization"),
	}

	if !s.enableEvents {
		log.Warn("event payloads are disabled: all events will be dropped")
	}
	if !s.enableSeries {
		log.Warn("series payloads are disabled: all series will be dropped")
	}
	if !s.enableServiceChecks {
		log.Warn("service_checks payloads are disabled: all service_checks will be dropped")
	}
	if !s.enableSketches {
		log.Warn("sketches payloads are disabled: all sketches will be dropped")
	}
	if !s.enableJSONToV1Intake {
		log.Warn("JSON to V1 intake is disabled: all payloads to that endpoint will be dropped")
	}

	return s
}

func (s Serializer) serializePayload(payload marshaler.Marshaler, compress bool, useV1API bool) (forwarder.Payloads, http.Header, error) {
	var marshalType split.MarshalType
	var extraHeaders http.Header

	if useV1API {
		marshalType = split.MarshalJSON
		if compress {
			extraHeaders = jsonExtraHeadersWithCompression
		} else {
			extraHeaders = jsonExtraHeaders
		}
	} else {
		marshalType = split.Marshal
		if compress {
			extraHeaders = protobufExtraHeadersWithCompression
		} else {
			extraHeaders = protobufExtraHeaders
		}
	}

	payloads, err := split.Payloads(payload, compress, marshalType)

	if err != nil {
		return nil, nil, fmt.Errorf("could not split payload into small enough chunks: %s", err)
	}

	return payloads, extraHeaders, nil
}

func (s Serializer) serializeStreamablePayload(payload marshaler.StreamJSONMarshaler, policy jsonstream.OnErrItemTooBigPolicy) (forwarder.Payloads, http.Header, error) {
	payloads, err := s.seriesPayloadBuilder.BuildWithOnErrItemTooBigPolicy(payload, policy)
	return payloads, jsonExtraHeadersWithCompression, err
}

func (s Serializer) serializeStreamJSONMarshalerFactoryPayload(
	streamJSONMarshalerFactory marshaler.StreamJSONMarshalerFactory) (forwarder.Payloads, http.Header, error) {
	marshaler := streamJSONMarshalerFactory.CreateSingleMarshaler()
	eventPayloads, extraHeaders, err := s.serializeStreamablePayload(marshaler, jsonstream.FailedErrItemTooBig)

	if err == jsonstream.ErrItemTooBig {
		for _, v := range streamJSONMarshalerFactory.CreateMarshalerCollection() {
			var eventPayloadsForSourceType forwarder.Payloads
			eventPayloadsForSourceType, extraHeaders, err = s.serializeStreamablePayload(v, jsonstream.ContinueOnErrItemTooBig)
			if err != nil {
				return nil, nil, err
			}
			eventPayloads = append(eventPayloads, eventPayloadsForSourceType...)
		}
	}
	return eventPayloads, extraHeaders, err
}

// SendEvents serializes a list of event and sends the payload to the forwarder
func (s *Serializer) SendEvents(e marshaler.StreamJSONMarshalerFactory) error {
	if !s.enableEvents {
		log.Debug("events payloads are disabled: dropping it")
		return nil
	}

	useV1API := !config.Datadog.GetBool("use_v2_api.events")
	var eventPayloads forwarder.Payloads
	var extraHeaders http.Header
	var err error

	if useV1API && s.enableEventsJSONStream {
		eventPayloads, extraHeaders, err = s.serializeStreamJSONMarshalerFactoryPayload(e)
	} else {
		eventPayloads, extraHeaders, err = s.serializePayload(e, true, useV1API)
	}
	if err != nil {
		return fmt.Errorf("dropping event payload: %s", err)
	}

	if useV1API {
		return s.Forwarder.SubmitV1Intake(eventPayloads, extraHeaders)
	}
	return s.Forwarder.SubmitEvents(eventPayloads, extraHeaders)
}

// SendServiceChecks serializes a list of serviceChecks and sends the payload to the forwarder
func (s *Serializer) SendServiceChecks(sc marshaler.StreamJSONMarshaler) error {
	if !s.enableServiceChecks {
		log.Debug("service_checks payloads are disabled: dropping it")
		return nil
	}

	useV1API := !config.Datadog.GetBool("use_v2_api.service_checks")

	var serviceCheckPayloads forwarder.Payloads
	var extraHeaders http.Header
	var err error

	if useV1API && s.enableServiceChecksJSONStream {
		serviceCheckPayloads, extraHeaders, err = s.serializeStreamablePayload(sc, jsonstream.ContinueOnErrItemTooBig)
	} else {
		serviceCheckPayloads, extraHeaders, err = s.serializePayload(sc, true, useV1API)
	}
	if err != nil {
		return fmt.Errorf("dropping service check payload: %s", err)
	}

	if useV1API {
		return s.Forwarder.SubmitV1CheckRuns(serviceCheckPayloads, extraHeaders)
	}
	return s.Forwarder.SubmitServiceChecks(serviceCheckPayloads, extraHeaders)
}

// SendSeries serializes a list of serviceChecks and sends the payload to the forwarder
func (s *Serializer) SendSeries(series marshaler.StreamJSONMarshaler) error {
	if !s.enableSeries {
		log.Debug("series payloads are disabled: dropping it")
		return nil
	}

	useV1API := !config.Datadog.GetBool("use_v2_api.series")

	var seriesPayloads forwarder.Payloads
	var extraHeaders http.Header
	var err error

	if useV1API && s.enableJSONStream {
		seriesPayloads, extraHeaders, err = s.serializeStreamablePayload(series, jsonstream.ContinueOnErrItemTooBig)
	} else {
		seriesPayloads, extraHeaders, err = s.serializePayload(series, true, useV1API)
	}

	if err != nil {
		return fmt.Errorf("dropping series payload: %s", err)
	}

	if useV1API {
		return s.Forwarder.SubmitV1Series(seriesPayloads, extraHeaders)
	}
	return s.Forwarder.SubmitSeries(seriesPayloads, extraHeaders)
}

// SendSketch serializes a list of SketSeriesList and sends the payload to the forwarder
func (s *Serializer) SendSketch(sketches marshaler.Marshaler) error {
	if !s.enableSketches {
		log.Debug("sketches payloads are disabled: dropping it")
		return nil
	}

	compress := true
	useV1API := false // Sketches only have a v2 endpoint
	splitSketches, extraHeaders, err := s.serializePayload(sketches, compress, useV1API)
	if err != nil {
		return fmt.Errorf("dropping sketch payload: %s", err)
	}

	return s.Forwarder.SubmitSketchSeries(splitSketches, extraHeaders)
}

// SendMetadata serializes a metadata payload and sends it to the forwarder
func (s *Serializer) SendMetadata(m marshaler.Marshaler) error {
	smallEnough, compressedPayload, payload, err := split.CheckSizeAndSerialize(m, true, split.MarshalJSON)
	if err != nil {
		return fmt.Errorf("could not determine size of metadata payload: %s", err)
	}

	log.Debugf("Sending host metadata payload, content: %v", apiKeyRegExp.ReplaceAllString(string(payload), apiKeyReplacement))

	if !smallEnough {
		return fmt.Errorf("metadata payload was too big to send (%d bytes compressed), metadata payloads cannot be split", len(compressedPayload))
	}

	if err := s.Forwarder.SubmitV1Intake(forwarder.Payloads{&compressedPayload}, jsonExtraHeadersWithCompression); err != nil {
		return err
	}

	log.Infof("Sent host metadata payload, size (raw/compressed): %d/%d bytes.", len(payload), len(compressedPayload))
	return nil
}

// SendJSONToV1Intake serializes a payload and sends it to the forwarder. Some code sends
// arbitrary payload the v1 API.
func (s *Serializer) SendJSONToV1Intake(data interface{}) error {
	if !s.enableJSONToV1Intake {
		log.Debug("JSON to V1 intake endpoint payloads are disabled: dropping it")
		return nil
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("could not serialize v1 payload: %s", err)
	}
	if err := s.Forwarder.SubmitV1Intake(forwarder.Payloads{&payload}, jsonExtraHeaders); err != nil {
		return err
	}

	log.Infof("Sent processes metadata payload, size: %d bytes.", len(payload))
	log.Debugf("Sent processes metadata payload, content: %v", apiKeyRegExp.ReplaceAllString(string(payload), apiKeyReplacement))
	return nil
}
