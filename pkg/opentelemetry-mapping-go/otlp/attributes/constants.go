// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package attributes provides attributes for the OpenTelemetry Collector.
package attributes

import (
	semconv1_17 "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv1_6_1 "go.opentelemetry.io/otel/semconv/v1.6.1"
)

// DefaultServiceName is the default service name for OTel spans when no service name is found in the resource attributes.
const DefaultServiceName = "unknown_service"

// DefaultEnvName is the default environment name for OTel spans when no environment name is found in the resource attributes.
const DefaultEnvName = "default"

// span.Type constants for db systems
const (
	SpanTypeSQL           = "sql"
	SpanTypeCassandra     = "cassandra"
	SpanTypeRedis         = "redis"
	SpanTypeMemcached     = "memcached"
	SpanTypeMongoDB       = "mongodb"
	SpanTypeElasticsearch = "elasticsearch"
	SpanTypeOpenSearch    = "opensearch"
	SpanTypeDB            = "db"
)

// DBTypes are semconv1_6_1 types that should map to span.Type values given in the mapping
var DBTypes = map[string]string{
	// SQL db types
	semconv1_6_1.DBSystemOtherSQL.Value.AsString():    SpanTypeSQL,
	semconv1_6_1.DBSystemMSSQL.Value.AsString():       SpanTypeSQL,
	semconv1_6_1.DBSystemMySQL.Value.AsString():       SpanTypeSQL,
	semconv1_6_1.DBSystemOracle.Value.AsString():      SpanTypeSQL,
	semconv1_6_1.DBSystemDB2.Value.AsString():         SpanTypeSQL,
	semconv1_6_1.DBSystemPostgreSQL.Value.AsString():  SpanTypeSQL,
	semconv1_6_1.DBSystemRedshift.Value.AsString():    SpanTypeSQL,
	semconv1_6_1.DBSystemCloudscape.Value.AsString():  SpanTypeSQL,
	semconv1_6_1.DBSystemHSQLDB.Value.AsString():      SpanTypeSQL,
	semconv1_6_1.DBSystemMaxDB.Value.AsString():       SpanTypeSQL,
	semconv1_6_1.DBSystemIngres.Value.AsString():      SpanTypeSQL,
	semconv1_6_1.DBSystemFirstSQL.Value.AsString():    SpanTypeSQL,
	semconv1_6_1.DBSystemEDB.Value.AsString():         SpanTypeSQL,
	semconv1_6_1.DBSystemCache.Value.AsString():       SpanTypeSQL,
	semconv1_6_1.DBSystemFirebird.Value.AsString():    SpanTypeSQL,
	semconv1_6_1.DBSystemDerby.Value.AsString():       SpanTypeSQL,
	semconv1_6_1.DBSystemInformix.Value.AsString():    SpanTypeSQL,
	semconv1_6_1.DBSystemMariaDB.Value.AsString():     SpanTypeSQL,
	semconv1_6_1.DBSystemSqlite.Value.AsString():      SpanTypeSQL,
	semconv1_6_1.DBSystemSybase.Value.AsString():      SpanTypeSQL,
	semconv1_6_1.DBSystemTeradata.Value.AsString():    SpanTypeSQL,
	semconv1_6_1.DBSystemVertica.Value.AsString():     SpanTypeSQL,
	semconv1_6_1.DBSystemH2.Value.AsString():          SpanTypeSQL,
	semconv1_6_1.DBSystemColdfusion.Value.AsString():  SpanTypeSQL,
	semconv1_6_1.DBSystemCockroachdb.Value.AsString(): SpanTypeSQL,
	semconv1_6_1.DBSystemProgress.Value.AsString():    SpanTypeSQL,
	semconv1_6_1.DBSystemHanaDB.Value.AsString():      SpanTypeSQL,
	semconv1_6_1.DBSystemAdabas.Value.AsString():      SpanTypeSQL,
	semconv1_6_1.DBSystemFilemaker.Value.AsString():   SpanTypeSQL,
	semconv1_6_1.DBSystemInstantDB.Value.AsString():   SpanTypeSQL,
	semconv1_6_1.DBSystemInterbase.Value.AsString():   SpanTypeSQL,
	semconv1_6_1.DBSystemNetezza.Value.AsString():     SpanTypeSQL,
	semconv1_6_1.DBSystemPervasive.Value.AsString():   SpanTypeSQL,
	semconv1_6_1.DBSystemPointbase.Value.AsString():   SpanTypeSQL,
	semconv1_17.DBSystemClickhouse.Value.AsString():   SpanTypeSQL, // not in semconv1_6_1 1.6.1

	// Cassandra db types
	semconv1_6_1.DBSystemCassandra.Value.AsString(): SpanTypeCassandra,

	// Redis db types
	semconv1_6_1.DBSystemRedis.Value.AsString(): SpanTypeRedis,

	// Memcached db types
	semconv1_6_1.DBSystemMemcached.Value.AsString(): SpanTypeMemcached,

	// Mongodb db types
	semconv1_6_1.DBSystemMongoDB.Value.AsString(): SpanTypeMongoDB,

	// Elasticsearch db types
	semconv1_6_1.DBSystemElasticsearch.Value.AsString(): SpanTypeElasticsearch,

	// Opensearch db types, not in semconv1_6_1 1.6.1
	semconv1_17.DBSystemOpensearch.Value.AsString(): SpanTypeOpenSearch,

	// Generic db types
	semconv1_6_1.DBSystemHive.Value.AsString():      SpanTypeDB,
	semconv1_6_1.DBSystemHBase.Value.AsString():     SpanTypeDB,
	semconv1_6_1.DBSystemNeo4j.Value.AsString():     SpanTypeDB,
	semconv1_6_1.DBSystemCouchbase.Value.AsString(): SpanTypeDB,
	semconv1_6_1.DBSystemCouchDB.Value.AsString():   SpanTypeDB,
	semconv1_6_1.DBSystemCosmosDB.Value.AsString():  SpanTypeDB,
	semconv1_6_1.DBSystemDynamoDB.Value.AsString():  SpanTypeDB,
	semconv1_6_1.DBSystemGeode.Value.AsString():     SpanTypeDB,
}
