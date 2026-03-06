// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package semantics provides a registry for semantic attribute equivalences
// across different tracing conventions (Datadog tracers, OpenTelemetry).
//
// Future work (OTel semantic convention updates):
//   - rpc.service is deprecated; the replacement is to include it as part of rpc.method,
//     so the fallback system alone cannot extract the concept value. Needs different handling.
//   - rpc.system is superseded by rpc.system.name; add rpc.system.name as fallback/canonical.
//   - db.system is deprecated in favor of db.system.name; add db.system.name to mappings.
package semantics

// Provider indicates the source of a semantic attribute definition.
type Provider string

const (
	ProviderDatadog Provider = "datadog"
	ProviderOTel    Provider = "otel"
)

// ValueType indicates the type of the attribute value.
type ValueType string

const (
	ValueTypeString  ValueType = "string"
	ValueTypeFloat64 ValueType = "float64"
	ValueTypeInt64   ValueType = "int64"
)

// Concept is an integer identifier for a semantic concept.
// Using int (rather than string) allows the registry to use a slice instead of
// a hash map, reducing GetAttributePrecedence to a single array index.
type Concept int

// Concept constants. The order here determines the index used in the registry
// slice — do not reorder without also updating conceptNames below.
const (
	// Peer Tags (used for stats aggregation)
	ConceptPeerService Concept = iota
	ConceptPeerHostname
	ConceptPeerDBName
	ConceptPeerDBSystem
	ConceptPeerCassandraContactPts
	ConceptPeerCouchbaseSeedNodes
	ConceptPeerMessagingDestination
	ConceptPeerMessagingSystem
	ConceptPeerKafkaBootstrapSrvs
	ConceptPeerRPCService
	ConceptPeerRPCSystem
	ConceptPeerAWSS3Bucket
	ConceptPeerAWSSQSQueue
	ConceptPeerAWSDynamoDBTable
	ConceptPeerAWSKinesisStream

	// Stats Aggregation
	ConceptHTTPStatusCode
	ConceptHTTPMethod
	ConceptHTTPRoute
	ConceptGRPCStatusCode
	ConceptSpanKind
	ConceptDDBaseService

	// Service & Resource Identification
	ConceptServiceName
	ConceptResourceName
	ConceptOperationName
	ConceptSpanType
	ConceptDBSystem
	ConceptDBStatement
	ConceptDBNamespace
	ConceptRPCSystem
	ConceptRPCService
	ConceptMessagingSystem
	ConceptMessagingDest
	ConceptDeploymentEnv
	ConceptServiceVersion
	ConceptContainerID
	ConceptK8sPodUID

	// Obfuscation
	ConceptDBQuery
	ConceptMongoDBQuery
	ConceptElasticsearchBody
	ConceptOpenSearchBody
	ConceptRedisRawCommand
	ConceptValkeyRawCommand
	ConceptMemcachedCommand
	ConceptHTTPURL

	// Normalization
	ConceptMessagingOperation
	ConceptGraphQLOperationType
	ConceptGraphQLOperationName
	ConceptFaaSInvokedProvider
	ConceptFaaSInvokedName
	ConceptFaaSTrigger
	ConceptNetworkProtocolName
	ConceptRPCMethod
	ConceptComponent
	ConceptLinkName

	// Sampling
	ConceptDDMeasured
	ConceptDDTopLevel
	ConceptSamplingPriority
	ConceptOTelTraceID
	ConceptDDTraceIDHigh
	ConceptDDPartialVersion

	// conceptCount must remain last — it is the total number of concepts and
	// is used to size the registry slice.
	conceptCount
)

// conceptNames maps each Concept to its canonical name in mappings.json.
// Must stay in sync with the iota constants above.
var conceptNames = [conceptCount]string{
	ConceptPeerService:              "peer.service",
	ConceptPeerHostname:             "peer.hostname",
	ConceptPeerDBName:               "peer.db.name",
	ConceptPeerDBSystem:             "peer.db.system",
	ConceptPeerCassandraContactPts:  "peer.cassandra.contact.points",
	ConceptPeerCouchbaseSeedNodes:   "peer.couchbase.seed.nodes",
	ConceptPeerMessagingDestination: "peer.messaging.destination",
	ConceptPeerMessagingSystem:      "peer.messaging.system",
	ConceptPeerKafkaBootstrapSrvs:   "peer.kafka.bootstrap.servers",
	ConceptPeerRPCService:           "peer.rpc.service",
	ConceptPeerRPCSystem:            "peer.rpc.system",
	ConceptPeerAWSS3Bucket:          "peer.aws.s3.bucket",
	ConceptPeerAWSSQSQueue:          "peer.aws.sqs.queue",
	ConceptPeerAWSDynamoDBTable:     "peer.aws.dynamodb.table",
	ConceptPeerAWSKinesisStream:     "peer.aws.kinesis.stream",

	ConceptHTTPStatusCode: "http.status_code",
	ConceptHTTPMethod:     "http.method",
	ConceptHTTPRoute:      "http.route",
	ConceptGRPCStatusCode: "rpc.grpc.status_code",
	ConceptSpanKind:       "span.kind",
	ConceptDDBaseService:  "_dd.base_service",

	ConceptServiceName:     "service.name",
	ConceptResourceName:    "resource.name",
	ConceptOperationName:   "operation.name",
	ConceptSpanType:        "span.type",
	ConceptDBSystem:        "db.system",
	ConceptDBStatement:     "db.statement",
	ConceptDBNamespace:     "db.namespace",
	ConceptRPCSystem:       "rpc.system",
	ConceptRPCService:      "rpc.service",
	ConceptMessagingSystem: "messaging.system",
	ConceptMessagingDest:   "messaging.destination",
	ConceptDeploymentEnv:   "deployment.environment",
	ConceptServiceVersion:  "service.version",
	ConceptContainerID:     "container.id",
	ConceptK8sPodUID:       "k8s.pod.uid",

	ConceptDBQuery:           "db.query",
	ConceptMongoDBQuery:      "mongodb.query",
	ConceptElasticsearchBody: "elasticsearch.body",
	ConceptOpenSearchBody:    "opensearch.body",
	ConceptRedisRawCommand:   "redis.raw_command",
	ConceptValkeyRawCommand:  "valkey.raw_command",
	ConceptMemcachedCommand:  "memcached.command",
	ConceptHTTPURL:           "http.url",

	ConceptMessagingOperation:   "messaging.operation",
	ConceptGraphQLOperationType: "graphql.operation.type",
	ConceptGraphQLOperationName: "graphql.operation.name",
	ConceptFaaSInvokedProvider:  "faas.invoked.provider",
	ConceptFaaSInvokedName:      "faas.invoked.name",
	ConceptFaaSTrigger:          "faas.trigger",
	ConceptNetworkProtocolName:  "network.protocol.name",
	ConceptRPCMethod:            "rpc.method",
	ConceptComponent:            "component",
	ConceptLinkName:             "link.name",

	ConceptDDMeasured:       "_dd.measured",
	ConceptDDTopLevel:       "_dd.top_level",
	ConceptSamplingPriority: "_sampling_priority_v1",
	ConceptOTelTraceID:      "otel.trace_id",
	ConceptDDTraceIDHigh:    "_dd.p.tid",
	ConceptDDPartialVersion: "_dd.partial_version",
}

// TagInfo contains metadata about a semantic attribute and its location.
type TagInfo struct {
	Name     string    `json:"name"`
	Provider Provider  `json:"provider"`
	Version  string    `json:"version,omitempty"`
	Type     ValueType `json:"type,omitempty"`
}

// ConceptMapping represents a semantic concept and its equivalent attributes.
type ConceptMapping struct {
	Canonical string    `json:"canonical"`
	Fallbacks []TagInfo `json:"fallbacks"`
}

// Registry provides access to semantic equivalences for span attributes.
type Registry interface {
	// GetAttributePrecedence returns ordered list of attribute keys to check
	GetAttributePrecedence(concept Concept) []TagInfo

	// GetAllEquivalences returns all semantic equivalences as a map from concept to the ordered list of equivalent attribute keys.
	GetAllEquivalences() map[Concept][]TagInfo

	// Version returns the semantic registry version string.
	Version() string
}
