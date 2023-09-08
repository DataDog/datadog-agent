// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"

	agentmodel "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

type Connections struct {
	agentmodel.CollectorConnections
}

func (c *Connections) name() string {
	return c.HostName + c.NetworkId
}

func (c *Connections) GetTags() []string {
	return c.GetTags()
}

func DecodeCollectorConnection(b []byte) (cnx *agentmodel.CollectorConnections, err error) {
	m, err := agentmodel.DecodeMessage(b)
	if err != nil {
		return nil, err
	}
	conns, ok := m.Body.(*agentmodel.CollectorConnections)
	if !ok {
		return nil, fmt.Errorf("not protobuf process.CollectorConnections type")
	}
	return conns, nil
}

func ParseConnections(payload api.Payload) (conns []*Connections, err error) {
	connections, err := DecodeCollectorConnection(payload.Data)
	if err != nil {
		return nil, err
	}
	var cnx []*Connections
	// we don't aggregate Connections but CollectorConnections for the moment
	cnx = append(cnx, &Connections{CollectorConnections: *connections})
	return cnx, nil
}

type ConnectionsAggregator struct {
	Aggregator[*Connections]
}

func NewConnectionsAggregator() ConnectionsAggregator {
	return ConnectionsAggregator{
		Aggregator: newAggregator(ParseConnections),
	}
}
