// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package metrics

import (
	"bytes"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gogo/protobuf/proto"
	jsoniter "github.com/json-iterator/go"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	utiljson "github.com/DataDog/datadog-agent/pkg/util/json"
)

// ServiceCheckStatus represents the status associated with a service check
type ServiceCheckStatus int

// Enumeration of the existing service check statuses, and their values
const (
	ServiceCheckOK       ServiceCheckStatus = iota
	ServiceCheckWarning  ServiceCheckStatus = 1
	ServiceCheckCritical ServiceCheckStatus = 2
	ServiceCheckUnknown  ServiceCheckStatus = 3
)

var (
	serviceCheckExpvar = expvar.NewMap("ServiceCheck")

	tlmServiceCheck = telemetry.NewCounter("metrics", "service_check_split",
		[]string{"action"}, "Service check split")
)

// GetServiceCheckStatus returns the ServiceCheckStatus from and integer value
func GetServiceCheckStatus(val int) (ServiceCheckStatus, error) {
	switch val {
	case int(ServiceCheckOK):
		return ServiceCheckOK, nil
	case int(ServiceCheckWarning):
		return ServiceCheckWarning, nil
	case int(ServiceCheckCritical):
		return ServiceCheckCritical, nil
	case int(ServiceCheckUnknown):
		return ServiceCheckUnknown, nil
	default:
		return ServiceCheckUnknown, fmt.Errorf("invalid value for a ServiceCheckStatus")
	}
}

// String returns a string representation of ServiceCheckStatus
func (s ServiceCheckStatus) String() string {
	switch s {
	case ServiceCheckOK:
		return "OK"
	case ServiceCheckWarning:
		return "WARNING"
	case ServiceCheckCritical:
		return "CRITICAL"
	case ServiceCheckUnknown:
		return "UNKNOWN"
	default:
		return ""
	}
}

// ServiceCheck holds a service check (w/ serialization to DD api format)
type ServiceCheck struct {
	CheckName   string             `json:"check"`
	Host        string             `json:"host_name"`
	Ts          int64              `json:"timestamp"`
	Status      ServiceCheckStatus `json:"status"`
	Message     string             `json:"message"`
	Tags        []string           `json:"tags"`
	OriginID    string             `json:"-"`
	K8sOriginID string             `json:"-"`
}

// ServiceChecks represents a list of service checks ready to be serialize
type ServiceChecks []*ServiceCheck

// Marshal serialize service checks using agent-payload definition
func (sc ServiceChecks) Marshal() ([]byte, error) {
	payload := &agentpayload.ServiceChecksPayload{
		ServiceChecks: []*agentpayload.ServiceChecksPayload_ServiceCheck{},
		Metadata:      &agentpayload.CommonMetadata{},
	}

	for _, c := range sc {
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

	return proto.Marshal(payload)
}

// MarshalJSON serializes service checks to JSON so it can be sent to V1 endpoints
//FIXME(olivier): to be removed when v2 endpoints are available
func (sc ServiceChecks) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinite recursion while serializing
	type ServiceChecksAlias ServiceChecks

	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(ServiceChecksAlias(sc))
	return reqBody.Bytes(), err
}

// MarshalStrings converts the service checks to a sorted slice of string slices
func (sc ServiceChecks) MarshalStrings() ([]string, [][]string) {
	var headers = []string{"Check", "Hostname", "Timestamp", "Status", "Message", "Tags"}
	var payload = make([][]string, len(sc))

	for _, c := range sc {
		payload = append(payload, []string{
			c.CheckName,
			c.Host,
			strconv.FormatInt(c.Ts, 10),
			c.Status.String(),
			c.Message,
			strings.Join(c.Tags, ", "),
		})
	}

	sort.Slice(payload, func(i, j int) bool {
		// edge cases
		if len(payload[i]) == 0 && len(payload[j]) == 0 {
			return false
		}
		if len(payload[i]) == 0 || len(payload[j]) == 0 {
			return len(payload[i]) == 0
		}
		// sort by service check name
		if payload[i][0] != payload[j][0] {
			return payload[i][0] < payload[j][0]
		}
		// then by timestamp
		if payload[i][2] != payload[j][2] {
			return payload[i][2] < payload[j][2]
		}
		// finally by tags (last field) as tie breaker
		return payload[i][len(payload[i])-1] < payload[j][len(payload[j])-1]
	})

	return headers, payload
}

// SplitPayload breaks the payload into times number of pieces
func (sc ServiceChecks) SplitPayload(times int) ([]marshaler.Marshaler, error) {
	serviceCheckExpvar.Add("TimesSplit", 1)
	tlmServiceCheck.Inc("times_split")
	// only split it up as much as possible
	if len(sc) < times {
		serviceCheckExpvar.Add("ServiceChecksShorter", 1)
		tlmServiceCheck.Inc("shorter")
		times = len(sc)
	}
	splitPayloads := make([]marshaler.Marshaler, times)
	batchSize := len(sc) / times
	n := 0
	for i := 0; i < times; i++ {
		var end int
		// the batch size will not be perfect, only split it as much as possible
		if i < times-1 {
			end = n + batchSize
		} else {
			end = len(sc)
		}
		newSC := sc[n:end]
		splitPayloads[i] = newSC
		n += batchSize
	}
	return splitPayloads, nil
}

func (sc ServiceCheck) String() string {
	s, err := json.Marshal(sc)
	if err != nil {
		return ""
	}
	return string(s)
}

//// The following methods implement the StreamJSONMarshaler interface
//// for support of the enable_service_checks_stream_payload_serialization option.

// WriteHeader writes the payload header for this type
func (sc ServiceChecks) WriteHeader(stream *jsoniter.Stream) error {
	stream.WriteArrayStart()
	return stream.Flush()
}

// WriteFooter prints the payload footer for this type
func (sc ServiceChecks) WriteFooter(stream *jsoniter.Stream) error {
	stream.WriteArrayEnd()
	return stream.Flush()
}

// WriteItem prints the json representation of an item
func (sc ServiceChecks) WriteItem(stream *jsoniter.Stream, i int) error {
	if i < 0 || i > len(sc)-1 {
		return errors.New("out of range")
	}

	if err := writeServiceCheck(sc[i], stream); err != nil {
		return err
	}
	return stream.Flush()
}

// Len returns the number of items to marshal
func (sc ServiceChecks) Len() int {
	return len(sc)
}

// DescribeItem returns a text description for logs
func (sc ServiceChecks) DescribeItem(i int) string {
	if i < 0 || i > len(sc)-1 {
		return "out of range"
	}
	return fmt.Sprintf("CheckName:%q, Message:%q", sc[i].CheckName, sc[i].Message)
}

func writeServiceCheck(sc *ServiceCheck, stream *jsoniter.Stream) error {
	writer := utiljson.NewRawObjectWriter(stream)

	if err := writer.StartObject(); err != nil {
		return err
	}
	writer.AddStringField("check", sc.CheckName, utiljson.AllowEmpty)
	writer.AddStringField("host_name", sc.Host, utiljson.AllowEmpty)
	writer.AddInt64Field("timestamp", sc.Ts)
	writer.AddInt64Field("status", int64(sc.Status))
	writer.AddStringField("message", sc.Message, utiljson.AllowEmpty)

	tagsField := "tags"

	if len(sc.Tags) == 0 {
		stream.WriteMore()
		stream.WriteObjectField(tagsField)
		stream.WriteNil()
	} else {
		if err := writer.StartArrayField(tagsField); err != nil {
			return err
		}
		for _, tag := range sc.Tags {
			writer.AddStringValue(tag)
		}
		if err := writer.FinishArrayField(); err != nil {
			return err
		}
	}
	if err := writer.FinishObject(); err != nil {
		return err
	}
	return writer.Flush()
}
