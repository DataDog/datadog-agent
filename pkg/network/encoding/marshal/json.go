// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

import (
	"bytes"
	"io"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/gogo/protobuf/jsonpb"

	"github.com/DataDog/datadog-agent/pkg/network"
)

// ContentTypeJSON holds the HTML content-type of a JSON payload
const ContentTypeJSON = "application/json"

type jsonSerializer struct {
	marshaller jsonpb.Marshaler
}

func (j jsonSerializer) Marshal(conns *network.Connections, writer io.Writer, connsModeler *ConnectionsModeler) error {
	out := bytes.NewBuffer(nil)
	connsModeler.modelConnections(model.NewConnectionsBuilder(out), conns)

	var payload model.Connections
	if err := payload.Unmarshal(out.Bytes()); err != nil {
		return err
	}

	return j.marshaller.Marshal(writer, &payload)
}

func (j jsonSerializer) ContentType() string {
	return ContentTypeJSON
}

var _ Marshaler = jsonSerializer{}
