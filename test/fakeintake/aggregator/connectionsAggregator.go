// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// Connections type contain all payload from /api/v1/connections
type Connections struct {
	agentmodel.CollectorConnections
	collectedTime time.Time
}

// name return connection payload name based on hostname and network ID
func (c *Connections) name() string {
	return c.HostName + "/" + c.NetworkId
}

// GetTags return tags connection
func (c *Connections) GetTags() []string {
	dns, err := c.GetDNSNames()
	if err != nil {
		return []string{}
	}
	return dns
}

// GetCollectedTime return the time when the payload has been collected by the fakeintake server
func (c *Connections) GetCollectedTime() time.Time {
	return c.collectedTime
}

// decodeCollectorConnection return a CollectorConnections protobuf object from raw bytes
func decodeCollectorConnection(b []byte) (cnx *agentmodel.CollectorConnections, err error) {
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

// ParseConnections return the Connections from payload
func ParseConnections(payload api.Payload) (conns []*Connections, err error) {
	connections, err := decodeCollectorConnection(payload.Data)
	if err != nil {
		return nil, err
	}
	var cnx []*Connections
	// we don't aggregate Connections but CollectorConnections for the moment
	cnx = append(cnx, &Connections{CollectorConnections: *connections, collectedTime: payload.Timestamp})
	return cnx, nil
}

// ConnectionsAggregator aggregate connections
type ConnectionsAggregator struct {
	Aggregator[*Connections]
}

// ForeachConnection will call the callback for each connection per hostname/netID and CollectorConnections payloads
func (ca *ConnectionsAggregator) ForeachConnection(callback func(c *agentmodel.Connection, cc *agentmodel.CollectorConnections, hostname string)) {
	for _, hostname := range ca.GetNames() {
		for _, cc := range ca.GetPayloadsByName(hostname) {
			for _, c := range cc.Connections {
				callback(c, &cc.CollectorConnections, hostname)
			}
		}
	}
}

// NewConnectionsAggregator create a new aggregator
func NewConnectionsAggregator() ConnectionsAggregator {
	return ConnectionsAggregator{
		Aggregator: newAggregator(ParseConnections),
	}
}
