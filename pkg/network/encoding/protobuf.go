// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/gogo/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/network"
)

// ContentTypeProtobuf holds the HTML content-type of a Protobuf payload
const ContentTypeProtobuf = "application/protobuf"

type protoSerializer struct{}

func (p protoSerializer) Model(conns *network.Connections, modeler *Modeler) *model.Connections {
	return modelConnections(conns, modeler.httpEncoder, modeler.http2Encoder, modeler.kafkaEncoder)
}

func (p protoSerializer) InitModeler(conns *network.Connections) *Modeler {
	return &Modeler{
		httpEncoder:  NewHTTPEncoder(conns.HTTP),
		http2Encoder: NewHTTP2Encoder(conns.HTTP2),
		kafkaEncoder: NewKafkaEncoder(conns.Kafka),
	}
}

func (protoSerializer) Marshal(payload *model.Connections) ([]byte, error) {
	buf, err := proto.Marshal(payload)
	returnToPool(payload)
	return buf, err
}

func (protoSerializer) Unmarshal(blob []byte) (*model.Connections, error) {
	conns := new(model.Connections)
	if err := proto.Unmarshal(blob, conns); err != nil {
		return nil, err
	}
	return conns, nil
}

func (p protoSerializer) ContentType() string {
	return ContentTypeProtobuf
}

var _ Marshaler = protoSerializer{}
var _ Unmarshaler = protoSerializer{}
