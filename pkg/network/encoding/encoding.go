package encoding

import (
	"strings"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/gogo/protobuf/jsonpb"
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
	Marshal(conns *network.Connections) ([]byte, error)
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

// GetUnmarshaler returns the appropriate Unmarshaler based on the given content type
func GetUnmarshaler(ctype string) Unmarshaler {
	if strings.Contains(ctype, ContentTypeProtobuf) {
		return pSerializer
	}

	return jSerializer
}

func modelConnections(conns *network.Connections) *model.Connections {
	agentConns := make([]*model.Connection, len(conns.Conns))
	domainSet := make(map[string]int)
	routeIndex := make(map[string]RouteIdx)
	httpIndex := FormatHTTPStats(conns.HTTP)

	for i, conn := range conns.Conns {
		httpKey := http.NewKey(conn.Source, conn.Dest, conn.SPort, conn.DPort, "")
		httpAggregations := httpIndex[httpKey]
		agentConns[i] = FormatConnection(conn, domainSet, routeIndex, httpAggregations)
		delete(httpIndex, httpKey)
		returnHTTPStats(httpAggregations)
	}

	// Delete orphan entries (those that can't be associated to a TCP connection)
	// TODO: investigate the source of this (perf misses on the TCP codepath?)
	for _, httpAggregations := range httpIndex {
		returnHTTPStats(httpAggregations)
	}

	domains := make([]string, len(domainSet))
	for k, v := range domainSet {
		domains[v] = k
	}

	routes := make([]*model.Route, len(routeIndex))
	for _, v := range routeIndex {
		routes[v.Idx] = &v.Route
	}

	payload := connsPool.Get().(*model.Connections)
	payload.Conns = agentConns
	payload.Domains = domains
	payload.Dns = FormatDNS(conns.DNS)
	payload.ConnTelemetry = FormatConnTelemetry(conns.ConnTelemetry)
	payload.CompilationTelemetryByAsset = FormatCompilationTelemetry(conns.CompilationTelemetryByAsset)
	payload.Routes = routes

	return payload
}
