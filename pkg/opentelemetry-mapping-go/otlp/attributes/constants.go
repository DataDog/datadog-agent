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

// DBTypes are db.system attribute values that should map to span.Type values given in the mapping
var DBTypes = map[string]string{
	// SQL db types
	"other_sql":   SpanTypeSQL,
	"mssql":       SpanTypeSQL,
	"mysql":       SpanTypeSQL,
	"oracle":      SpanTypeSQL,
	"db2":         SpanTypeSQL,
	"postgresql":  SpanTypeSQL,
	"redshift":    SpanTypeSQL,
	"cloudscape":  SpanTypeSQL,
	"hsqldb":      SpanTypeSQL,
	"maxdb":       SpanTypeSQL,
	"ingres":      SpanTypeSQL,
	"firstsql":    SpanTypeSQL,
	"edb":         SpanTypeSQL,
	"cache":       SpanTypeSQL,
	"firebird":    SpanTypeSQL,
	"derby":       SpanTypeSQL,
	"informix":    SpanTypeSQL,
	"mariadb":     SpanTypeSQL,
	"sqlite":      SpanTypeSQL,
	"sybase":      SpanTypeSQL,
	"teradata":    SpanTypeSQL,
	"vertica":     SpanTypeSQL,
	"h2":          SpanTypeSQL,
	"coldfusion":  SpanTypeSQL,
	"cockroachdb": SpanTypeSQL,
	"progress":    SpanTypeSQL,
	"hanadb":      SpanTypeSQL,
	"adabas":      SpanTypeSQL,
	"filemaker":   SpanTypeSQL,
	"instantdb":   SpanTypeSQL,
	"interbase":   SpanTypeSQL,
	"netezza":     SpanTypeSQL,
	"pervasive":   SpanTypeSQL,
	"pointbase":   SpanTypeSQL,
	"clickhouse":  SpanTypeSQL,

	// Cassandra db types
	"cassandra": SpanTypeCassandra,

	// Redis db types
	"redis": SpanTypeRedis,

	// Memcached db types
	"memcached": SpanTypeMemcached,

	// Mongodb db types
	"mongodb": SpanTypeMongoDB,

	// Elasticsearch db types
	"elasticsearch": SpanTypeElasticsearch,

	// Opensearch db types
	"opensearch": SpanTypeOpenSearch,

	// Generic db types
	"hive":     SpanTypeDB,
	"hbase":    SpanTypeDB,
	"neo4j":    SpanTypeDB,
	"couchbase": SpanTypeDB,
	"couchdb":  SpanTypeDB,
	"cosmosdb": SpanTypeDB,
	"dynamodb": SpanTypeDB,
	"geode":    SpanTypeDB,
}
