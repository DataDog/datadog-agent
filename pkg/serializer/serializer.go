package serializer

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	log "github.com/cihub/seelog"
)

const (
	protobufContentType = "application/x-protobuf"
	jsonContentType     = "application/json"

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

// Marshaler is an interface for metrics that are able to serialize themselves to JSON and protobuf
type Marshaler interface {
	MarshalJSON() ([]byte, error)
	Marshal() ([]byte, error)
}

// Serializer serializes metrics to the correct format and routes the payloads to the correct endpoint in the Forwarder
type Serializer struct {
	Forwarder forwarder.Forwarder
}

// SendEvents serializes a list of event and sends the payload to the forwarder
func (s *Serializer) SendEvents(e Marshaler) error {
	payload, err := e.MarshalJSON()
	if err != nil {
		return fmt.Errorf("could not serialize events, %s", err)
	}
	return s.Forwarder.SubmitV1Intake(&payload, jsonExtraHeaders)
}

// SendServiceChecks serializes a list of serviceChecks and sends the payload to the forwarder
func (s *Serializer) SendServiceChecks(sc Marshaler) error {
	payload, err := sc.MarshalJSON()
	if err != nil {
		return fmt.Errorf("could not serialize service checks, %s", err)
	}
	return s.Forwarder.SubmitV1CheckRuns(&payload, jsonExtraHeaders)
}

// SendSeries serializes a list of serviceChecks and sends the payload to the forwarder
func (s *Serializer) SendSeries(series Marshaler) error {
	payload, err := series.MarshalJSON()
	if err != nil {
		return fmt.Errorf("could not serialize series: %s", err)
	}
	return s.Forwarder.SubmitV1Series(&payload, jsonExtraHeaders)
}

// SendSketch serializes a list of SketSeriesList and sends the payload to the forwarder
func (s *Serializer) SendSketch(sketches Marshaler) error {
	payload, err := sketches.Marshal()
	if err != nil {
		return fmt.Errorf("could not serialize sketches: %s", err)
	}
	return s.Forwarder.SubmitSketchSeries(&payload, protobufExtraHeaders)
}

// SendMetadata serializes a metadata payload and sends it to the forwarder
func (s *Serializer) SendMetadata(m Marshaler) error {
	payload, err := m.MarshalJSON()
	if err != nil {
		return fmt.Errorf("could not serialize metadata payload: %s", err)
	}

	if err := s.Forwarder.SubmitV1Intake(&payload, jsonExtraHeaders); err != nil {
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

	if err := s.Forwarder.SubmitV1Intake(&payload, jsonExtraHeaders); err != nil {
		return err
	}

	log.Infof("Sent processes metadata payload, size: %d bytes.", len(payload))
	log.Debugf("Sent processes metadata payload, content: %v", string(payload))
	return nil

}
