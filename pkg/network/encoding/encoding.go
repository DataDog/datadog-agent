// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"io"
	"strings"

	"github.com/gogo/protobuf/jsonpb"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
)

var (
	pSerializer = protoSerializer{}
	jSerializer = jsonSerializer{
		marshaller: jsonpb.Marshaler{
			EmitDefaults: true,
		},
	}
)

// Marshaler is an interface implemented by all Connections serializers
type Marshaler interface {
	Marshal(conns *network.Connections, writer io.Writer, connsModeler *ConnectionsModeler) error
	ContentType() string
}

// Unmarshaler is an interface implemented by all Connections deserializers
type Unmarshaler interface {
	Unmarshal([]byte) (*model.Connections, error)
}

// GetMarshaler returns the appropriate Marshaler based on the given accept header
func GetMarshaler(accept string) Marshaler {
	if strings.Contains(accept, ContentTypeProtobuf) {
		return pSerializer
	}

	return jSerializer
}

// ConnectionsModeler contains all the necessary structs for modeling a connection.
type ConnectionsModeler struct {
	agentCfg     *model.AgentConfiguration
	httpEncoder  *httpEncoder
	http2Encoder *http2Encoder
	kafkaEncoder *kafkaEncoder
	dnsFormatter *dnsFormatter
	ipc          ipCache
	routeIndex   map[string]RouteIdx
	tagsSet      *network.TagsSet
	batchIndex   int
	batchCount   int
}

// NewConnectionsModeler initializes the connection modeler with encoders, dns formatter for
// the existing connections. The ConnectionsModeler holds the traffic encoders grouped by USM logic.
// It also includes formatted connection telemetry related to all batches, not specific batches.
// Furthermore, it stores the current agent configuration which applies to all instances related to the entire set of connections,
// rather than just individual batches.
func NewConnectionsModeler(conns *network.Connections) *ConnectionsModeler {
	ipc := make(ipCache, len(conns.Conns)/2)
	return &ConnectionsModeler{
		agentCfg: &model.AgentConfiguration{
			NpmEnabled: config.SystemProbe.GetBool("network_config.enabled"),
			UsmEnabled: config.SystemProbe.GetBool("service_monitoring_config.enabled"),
			DsmEnabled: config.SystemProbe.GetBool("data_streams_config.enabled"),
		},
		httpEncoder:  newHTTPEncoder(conns.HTTP),
		http2Encoder: newHTTP2Encoder(conns.HTTP2),
		kafkaEncoder: newKafkaEncoder(conns.Kafka),
		ipc:          ipc,
		dnsFormatter: newDNSFormatter(conns, ipc),
		routeIndex:   make(map[string]RouteIdx),
		tagsSet:      network.NewTagsSet(),
		batchCount:   1,
	}
}

// SetBatchCount sets the number of batches the modeler should have.
func (c *ConnectionsModeler) SetBatchCount(count int) {
	c.batchCount = count
}

// Close cleans all encoders resources.
func (c *ConnectionsModeler) Close() {
	c.httpEncoder.Close()
	c.http2Encoder.Close()
	c.kafkaEncoder.Close()
}

// GetUnmarshaler returns the appropriate Unmarshaler based on the given content type
func GetUnmarshaler(ctype string) Unmarshaler {
	if strings.Contains(ctype, ContentTypeProtobuf) {
		return pSerializer
	}

	return jSerializer
}

func (c *ConnectionsModeler) modelConnections(builder *model.ConnectionsBuilder, conns *network.Connections) {
	c.batchIndex++

	for _, conn := range conns.Conns {
		builder.AddConns(func(builder *model.ConnectionBuilder) {
			FormatConnection(builder, conn, c.routeIndex, c.httpEncoder, c.http2Encoder, c.kafkaEncoder, c.dnsFormatter, c.ipc, c.tagsSet)
		})
	}

	if c.batchCount == c.batchIndex {
		routes := make([]*model.Route, len(c.routeIndex))
		for _, v := range c.routeIndex {
			routes[v.Idx] = &v.Route
		}

		builder.SetAgentConfiguration(func(w *model.AgentConfigurationBuilder) {
			w.SetDsmEnabled(c.agentCfg.DsmEnabled)
			w.SetNpmEnabled(c.agentCfg.NpmEnabled)
			w.SetUsmEnabled(c.agentCfg.UsmEnabled)
		})
		for _, d := range c.dnsFormatter.Domains() {
			builder.AddDomains(d)
		}

		for _, route := range routes {
			builder.AddRoutes(func(w *model.RouteBuilder) {
				w.SetSubnet(func(w *model.SubnetBuilder) {
					w.SetAlias(route.Subnet.Alias)
				})
			})
		}

		c.dnsFormatter.FormatDNS(builder)

		for _, tag := range c.tagsSet.GetStrings() {
			builder.AddTags(tag)
		}

		FormatConnectionTelemetry(builder, conns.ConnTelemetry)
		FormatCompilationTelemetry(builder, conns.CompilationTelemetryByAsset)
		FormatCORETelemetry(builder, conns.CORETelemetryByAsset)
		builder.SetKernelHeaderFetchResult(uint64(conns.KernelHeaderFetchResult))
		for _, asset := range conns.PrebuiltAssets {
			builder.AddPrebuiltEBPFAssets(asset)
		}
	}
}
