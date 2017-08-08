package serializer

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/split"

	log "github.com/cihub/seelog"
)

const (
	protobufContentType      = "application/x-protobuf"
	jsonContentType          = "application/json"
	payloadVersionHTTPHeader = "DD-Agent-Payload"
)

var (
	// AgentPayloadVersion is the versions of the agent-payload repository
	// used to serialize to protobuf
	AgentPayloadVersion string

	jsonExtraHeaders     map[string]string
	protobufExtraHeaders map[string]string
)

func init() {
	jsonExtraHeaders = map[string]string{
		"Content-Type": jsonContentType,
	}

	protobufExtraHeaders = map[string]string{
		"Content-Type":           protobufContentType,
		payloadVersionHTTPHeader: AgentPayloadVersion,
	}
}

// Serializer serializes metrics to the correct format and routes the payloads to the correct endpoint in the Forwarder
type Serializer struct {
	Forwarder forwarder.Forwarder
}

func (s Serializer) splitPayload(payload marshaler.Marshaler, compress bool, useV1Endpoint bool) (forwarder.Payloads, error) {
	marshalType := split.Marshal
	if useV1Endpoint {
		marshalType = split.MarshalJSON
	}
	payloads, err := split.Payloads(payload, compress, marshalType)

	if err != nil {
		return nil, fmt.Errorf("could not split payload into small enough chunks: %s", err)
	}

	return payloads, nil
}

// SendEvents serializes a list of event and sends the payload to the forwarder
func (s *Serializer) SendEvents(e marshaler.Marshaler) error {
	useV1Endpoint := false
	if !config.Datadog.GetBool("use_v2_endpoint.events") {
		useV1Endpoint = true
	}

	compress := true
	eventPayloads, err := s.splitPayload(e, compress, useV1Endpoint)
	if err != nil {
		return fmt.Errorf("dropping event payload: %s", err)
	}

	if useV1Endpoint {
		return s.Forwarder.SubmitV1Intake(eventPayloads, jsonExtraHeaders)
	}
	return s.Forwarder.SubmitEvents(eventPayloads, protobufExtraHeaders)
}

// SendServiceChecks serializes a list of serviceChecks and sends the payload to the forwarder
func (s *Serializer) SendServiceChecks(sc marshaler.Marshaler) error {
	useV1Endpoint := false
	if !config.Datadog.GetBool("use_v2_endpoint.service_checks") {
		useV1Endpoint = true
	}

	compress := false
	serviceCheckPayloads, err := s.splitPayload(sc, compress, useV1Endpoint)
	if err != nil {
		return fmt.Errorf("dropping service check payload: %s", err)
	}

	if useV1Endpoint {
		return s.Forwarder.SubmitV1CheckRuns(serviceCheckPayloads, jsonExtraHeaders)
	}
	return s.Forwarder.SubmitServiceChecks(serviceCheckPayloads, protobufExtraHeaders)
}

// SendSeries serializes a list of serviceChecks and sends the payload to the forwarder
func (s *Serializer) SendSeries(series marshaler.Marshaler) error {
	useV1Endpoint := false
	if !config.Datadog.GetBool("use_v2_endpoint.series") {
		useV1Endpoint = true
	}

	compress := true
	seriesPayloads, err := s.splitPayload(series, compress, useV1Endpoint)
	if err != nil {
		return fmt.Errorf("dropping series payload: %s", err)
	}

	if useV1Endpoint {
		return s.Forwarder.SubmitV1Series(seriesPayloads, jsonExtraHeaders)
	}
	return s.Forwarder.SubmitSeries(seriesPayloads, protobufExtraHeaders)
}

// SendSketch serializes a list of SketSeriesList and sends the payload to the forwarder
func (s *Serializer) SendSketch(sketches marshaler.Marshaler) error {
	compress := false
	useV1Endpoint := false // Sketches only have a v2 endpoint
	splitSketches, err := s.splitPayload(sketches, compress, useV1Endpoint)
	if err != nil {
		return fmt.Errorf("dropping sketch payload: %s", err)
	}

	return s.Forwarder.SubmitSketchSeries(splitSketches, protobufExtraHeaders)
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
	log.Debugf("Sent host metadata payload, content: %v", string(payload))
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
	log.Debugf("Sent processes metadata payload, content: %v", string(payload))
	return nil
}
