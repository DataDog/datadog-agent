// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package serializer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
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

// Serializer serializes metrics to the correct format and routes the payloads to the correct endpoint in the Forwarder
type Serializer struct {
	Forwarder forwarder.Forwarder
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

// SendEvents serializes a list of event and sends the payload to the forwarder
func (s *Serializer) SendEvents(e marshaler.Marshaler) error {
	useV1API := !config.Datadog.GetBool("use_v2_api.events")

	compress := true
	eventPayloads, extraHeaders, err := s.serializePayload(e, compress, useV1API)
	if err != nil {
		return fmt.Errorf("dropping event payload: %s", err)
	}

	if useV1API {
		return s.Forwarder.SubmitV1Intake(eventPayloads, extraHeaders)
	}
	return s.Forwarder.SubmitEvents(eventPayloads, extraHeaders)
}

// SendServiceChecks serializes a list of serviceChecks and sends the payload to the forwarder
func (s *Serializer) SendServiceChecks(sc marshaler.Marshaler) error {
	useV1API := !config.Datadog.GetBool("use_v2_api.service_checks")

	compress := true
	serviceCheckPayloads, extraHeaders, err := s.serializePayload(sc, compress, useV1API)
	if err != nil {
		return fmt.Errorf("dropping service check payload: %s", err)
	}

	if useV1API {
		return s.Forwarder.SubmitV1CheckRuns(serviceCheckPayloads, extraHeaders)
	}
	return s.Forwarder.SubmitServiceChecks(serviceCheckPayloads, extraHeaders)
}

// SendSeries serializes a list of serviceChecks and sends the payload to the forwarder
func (s *Serializer) SendSeries(series marshaler.Marshaler) error {
	useV1API := !config.Datadog.GetBool("use_v2_api.series")

	compress := true
	seriesPayloads, extraHeaders, err := s.serializePayload(series, compress, useV1API)
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
	compress := false // TODO: enable compression once the backend supports it on this endpoint
	useV1API := false // Sketches only have a v2 endpoint
	splitSketches, extraHeaders, err := s.serializePayload(sketches, compress, useV1API)
	if err != nil {
		return fmt.Errorf("dropping sketch payload: %s", err)
	}

	return s.Forwarder.SubmitSketchSeries(splitSketches, extraHeaders)
}

// SendMetadata serializes a metadata payload and sends it to the forwarder
func (s *Serializer) SendMetadata(m marshaler.Marshaler) error {
	smallEnough, payload, err := split.CheckSizeAndSerialize(m, false, split.MarshalJSON)
	if err != nil {
		return fmt.Errorf("could not determine size of metadata payload: %s", err)
	} else if !smallEnough {
		return fmt.Errorf("metadata payload was too big to send, metadata payloads cannot be split")
	}

	if err := s.Forwarder.SubmitV1Intake(forwarder.Payloads{&payload}, jsonExtraHeaders); err != nil {
		return err
	}

	log.Infof("Sent host metadata payload, size: %d bytes.", len(payload))
	log.Debugf("Sent host metadata payload, content: %v", apiKeyRegExp.ReplaceAllString(string(payload), apiKeyReplacement))
	return nil
}

// SendJSONToV1Intake serializes a payload and sends it to the forwarder. Some code sends
// arbitrary payload the v1 API.
func (s *Serializer) SendJSONToV1Intake(data interface{}) error {
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
