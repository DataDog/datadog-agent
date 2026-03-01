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

// Concept represents a semantic concept identifier (e.g., "db.system", "http.method").
type Concept string

// Peer Tags (used for stats aggregation)
const (
	ConceptPeerService              Concept = "peer.service"
	ConceptPeerHostname             Concept = "peer.hostname"
	ConceptPeerDBName               Concept = "peer.db.name"
	ConceptPeerDBSystem             Concept = "peer.db.system"
	ConceptPeerCassandraContactPts  Concept = "peer.cassandra.contact.points"
	ConceptPeerCouchbaseSeedNodes   Concept = "peer.couchbase.seed.nodes"
	ConceptPeerMessagingDestination Concept = "peer.messaging.destination"
	ConceptPeerMessagingSystem      Concept = "peer.messaging.system"
	ConceptPeerKafkaBootstrapSrvs   Concept = "peer.kafka.bootstrap.servers"
	ConceptPeerRPCService           Concept = "peer.rpc.service"
	ConceptPeerRPCSystem            Concept = "peer.rpc.system"
	ConceptPeerAWSS3Bucket          Concept = "peer.aws.s3.bucket"
	ConceptPeerAWSSQSQueue          Concept = "peer.aws.sqs.queue"
	ConceptPeerAWSDynamoDBTable     Concept = "peer.aws.dynamodb.table"
	ConceptPeerAWSKinesisStream     Concept = "peer.aws.kinesis.stream"
)

// Stats Aggregation
const (
	ConceptHTTPStatusCode Concept = "http.status_code"
	ConceptHTTPMethod     Concept = "http.method"
	ConceptHTTPRoute      Concept = "http.route"
	ConceptGRPCStatusCode Concept = "rpc.grpc.status_code"
	ConceptSpanKind       Concept = "span.kind"
	ConceptDDBaseService  Concept = "_dd.base_service"
)

// Service & Resource Identification
const (
	ConceptServiceName     Concept = "service.name"
	ConceptResourceName    Concept = "resource.name"
	ConceptOperationName   Concept = "operation.name"
	ConceptSpanType        Concept = "span.type"
	ConceptDBSystem        Concept = "db.system"
	ConceptDBStatement     Concept = "db.statement"
	ConceptDBNamespace     Concept = "db.namespace"
	ConceptRPCSystem       Concept = "rpc.system"
	ConceptRPCService      Concept = "rpc.service"
	ConceptMessagingSystem Concept = "messaging.system"
	ConceptMessagingDest   Concept = "messaging.destination"
	ConceptDeploymentEnv   Concept = "deployment.environment"
	ConceptServiceVersion  Concept = "service.version"
	ConceptContainerID     Concept = "container.id"
	ConceptK8sPodUID       Concept = "k8s.pod.uid"
)

// Obfuscation
const (
	ConceptDBQuery           Concept = "db.query"
	ConceptMongoDBQuery      Concept = "mongodb.query"
	ConceptElasticsearchBody Concept = "elasticsearch.body"
	ConceptOpenSearchBody    Concept = "opensearch.body"
	ConceptRedisRawCommand   Concept = "redis.raw_command"
	ConceptValkeyRawCommand  Concept = "valkey.raw_command"
	ConceptMemcachedCommand  Concept = "memcached.command"
	ConceptHTTPURL           Concept = "http.url"
)

// Normalization
const (
	ConceptMessagingOperation   Concept = "messaging.operation"
	ConceptGraphQLOperationType Concept = "graphql.operation.type"
	ConceptGraphQLOperationName Concept = "graphql.operation.name"
	ConceptFaaSInvokedProvider  Concept = "faas.invoked.provider"
	ConceptFaaSInvokedName      Concept = "faas.invoked.name"
	ConceptFaaSTrigger          Concept = "faas.trigger"
	ConceptNetworkProtocolName  Concept = "network.protocol.name"
	ConceptRPCMethod            Concept = "rpc.method"
	ConceptComponent            Concept = "component"
	ConceptLinkName             Concept = "link.name"
)

// Sampling
const (
	ConceptDDMeasured       Concept = "_dd.measured"
	ConceptDDTopLevel       Concept = "_dd.top_level"
	ConceptSamplingPriority Concept = "_sampling_priority_v1"
	ConceptOTelTraceID      Concept = "otel.trace_id"
	ConceptDDTraceIDHigh    Concept = "_dd.p.tid"
	ConceptDDPartialVersion Concept = "_dd.partial_version"
)

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
