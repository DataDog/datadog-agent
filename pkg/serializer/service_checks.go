package serializer

import (
	"bytes"
	"encoding/json"

	"github.com/gogo/protobuf/proto"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// MarshalServiceChecks serialize check runs payload using agent-payload definition
func MarshalServiceChecks(checkRuns []*metrics.ServiceCheck) ([]byte, string, error) {
	payload := &agentpayload.ServiceChecksPayload{
		ServiceChecks: []*agentpayload.ServiceChecksPayload_ServiceCheck{},
		Metadata:      &agentpayload.CommonMetadata{},
	}

	for _, c := range checkRuns {
		payload.ServiceChecks = append(payload.ServiceChecks,
			&agentpayload.ServiceChecksPayload_ServiceCheck{
				Name:    c.CheckName,
				Host:    c.Host,
				Ts:      c.Ts,
				Status:  int32(c.Status),
				Message: c.Message,
				Tags:    c.Tags,
			})
	}

	msg, err := proto.Marshal(payload)
	return msg, protobufContentType, err
}

// MarshalJSONServiceChecks serializes service checks to JSON so it can be sent to V1 endpoints
//FIXME(olivier): to be removed when v2 endpoints are available
func MarshalJSONServiceChecks(serviceChecks []metrics.ServiceCheck) ([]byte, string, error) {
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(serviceChecks)
	return reqBody.Bytes(), jsonContentType, err
}
