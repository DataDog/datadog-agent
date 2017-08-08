package serializer

import (
	"encoding/json"
	"fmt"

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

// SendEvents serializes a list of event and sends the payload to the forwarder
func (s *Serializer) SendEvents(e marshaler.Marshaler) error {
	compress := true
	events, err := split.Payloads(e, compress, split.MarshalJSON)
	if err != nil {
		return fmt.Errorf("could not split events into small enough chunks, dropping: %s", err)
	}
	return s.Forwarder.SubmitV1Intake(events, jsonExtraHeaders)
}

// SendServiceChecks serializes a list of serviceChecks and sends the payload to the forwarder
func (s *Serializer) SendServiceChecks(sc marshaler.Marshaler) error {
	compress := false
	serviceChecks, err := split.Payloads(sc, compress, split.MarshalJSON)
	if err != nil {
		return fmt.Errorf("could not split service checks into small enough chunks, dropping: %s", err)
	}
	return s.Forwarder.SubmitV1CheckRuns(serviceChecks, jsonExtraHeaders)
}

// SendSeries serializes a list of serviceChecks and sends the payload to the forwarder
func (s *Serializer) SendSeries(series marshaler.Marshaler) error {
	compress := true

	splitSeries, err := split.Payloads(series, compress, split.MarshalJSON)
	if err != nil {
		return fmt.Errorf("could not split series into small enough chunks, dropping: %s", err)
	}
	return s.Forwarder.SubmitV1Series(splitSeries, jsonExtraHeaders)
}

// SendSketch serializes a list of SketSeriesList and sends the payload to the forwarder
func (s *Serializer) SendSketch(sketches marshaler.Marshaler) error {
	compress := false
	splitSketches, err := split.Payloads(sketches, compress, split.Marshal)
	if err != nil {
		return fmt.Errorf("could not split sketches into small enough chunks, dropping: %s", err)
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
