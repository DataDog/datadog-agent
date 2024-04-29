// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package unmarshal

import (
	"bytes"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/gogo/protobuf/jsonpb"
)

// ContentTypeJSON holds the HTML content-type of a JSON payload
const ContentTypeJSON = "application/json"

type jsonSerializer struct{}

func (j jsonSerializer) Unmarshal(blob []byte) (*model.Connections, error) {
	conns := new(model.Connections)
	reader := bytes.NewReader(blob)
	if err := jsonpb.Unmarshal(reader, conns); err != nil {
		return nil, err
	}

	handleZeroValues(conns)
	return conns, nil
}

func (j jsonSerializer) ContentType() string {
	return ContentTypeJSON
}

// this code is a hack to fix the way zero value maps are handled during a
// roundtrip (eg. marshaling/unmarshaling) by the JSON marshaler as we use the
// `EmitDefaults` option. please note this function is executed *only in
// tests* since the JSON unmarshaller is not used in the communication between
// system-probe and the process-agent.
// TODO: Make this more future-proof using reflection
func handleZeroValues(conns *model.Connections) {
	if conns == nil {
		return
	}

	if len(conns.CompilationTelemetryByAsset) == 0 {
		conns.CompilationTelemetryByAsset = nil
	}

	if len(conns.ConnTelemetryMap) == 0 {
		conns.ConnTelemetryMap = nil
	}

	if len(conns.CORETelemetryByAsset) == 0 {
		conns.CORETelemetryByAsset = nil
	}

	for _, c := range conns.Conns {
		if len(c.DnsCountByRcode) == 0 {
			c.DnsCountByRcode = nil
		}
		if len(c.DnsStatsByDomain) == 0 {
			c.DnsStatsByDomain = nil
		}
		if len(c.DnsStatsByDomainByQueryType) == 0 {
			c.DnsStatsByDomainByQueryType = nil
		}
		if len(c.DnsStatsByDomainOffsetByQueryType) == 0 {
			c.DnsStatsByDomainOffsetByQueryType = nil
		}
	}
}
