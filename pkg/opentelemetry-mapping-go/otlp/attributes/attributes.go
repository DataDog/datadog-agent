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
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv1_12 "go.opentelemetry.io/otel/semconv/v1.12.0"
	semconv1_17 "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv1_27 "go.opentelemetry.io/otel/semconv/v1.27.0"
	semconv1_6_1 "go.opentelemetry.io/otel/semconv/v1.6.1"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/util/normalize"
)

// TagsFromAttributes converts a selected list of attributes
// to a tag list that can be added to metrics.
func TagsFromAttributes(attrs pcommon.Map) []string {
	tags := make([]string, 0, attrs.Len())

	var processAttributes processAttributes
	var systemAttributes systemAttributes

	attrs.Range(func(key string, value pcommon.Value) bool {
		switch key {
		// Process attributes
		case string(semconv1_27.ProcessExecutableNameKey):
			processAttributes.ExecutableName = value.Str()
		case string(semconv1_27.ProcessExecutablePathKey):
			processAttributes.ExecutablePath = value.Str()
		case string(semconv1_27.ProcessCommandKey):
			processAttributes.Command = value.Str()
		case string(semconv1_27.ProcessCommandLineKey):
			processAttributes.CommandLine = value.Str()
		case string(semconv1_27.ProcessPIDKey):
			processAttributes.PID = value.Int()
		case string(semconv1_27.ProcessOwnerKey):
			processAttributes.Owner = value.Str()

		// System attributes
		case string(semconv1_27.OSTypeKey):
			systemAttributes.OSType = value.Str()
		}

		// core attributes mapping
		if datadogKey, found := coreMapping[key]; found && value.Str() != "" {
			tags = append(tags, fmt.Sprintf("%s:%s", datadogKey, value.Str()))
		}

		// Kubernetes labels mapping
		if datadogKey, found := kubernetesMapping[key]; found && value.Str() != "" {
			tags = append(tags, fmt.Sprintf("%s:%s", datadogKey, value.Str()))
		}

		// Kubernetes DD tags
		if _, found := kubernetesDDTags[key]; found {
			tags = append(tags, fmt.Sprintf("%s:%s", key, value.Str()))
		}
		return true
	})

	// Container Tag mappings
	ctags := ContainerTagsFromResourceAttributes(attrs)
	for key, val := range ctags {
		tags = append(tags, fmt.Sprintf("%s:%s", key, val))
	}

	tags = append(tags, processAttributes.extractTags()...)
	tags = append(tags, systemAttributes.extractTags()...)

	return tags
}

// OriginIDFromAttributes gets the origin IDs from resource attributes.
// If not found, an empty string is returned for each of them.
func OriginIDFromAttributes(attrs pcommon.Map) (originID string) {
	// originID is always empty. Container ID is preferred over Kubernetes pod UID.
	// Prefixes come from pkg/util/kubernetes/kubelet and pkg/util/containers.
	if containerID, ok := attrs.Get(string(semconv1_6_1.ContainerIDKey)); ok {
		originID = "container_id://" + containerID.AsString()
	} else if podUID, ok := attrs.Get(string(semconv1_6_1.K8SPodUIDKey)); ok {
		originID = "kubernetes_pod_uid://" + podUID.AsString()
	}
	return
}

// ContainerTagsFromResourceAttributes extracts container tags from the given
// set of resource attributes. Container tags are extracted via semantic
// conventions. Customer container tags are extracted via resource attributes
// prefixed by datadog.container.tag. Custom container tag values of a different type
// than ValueTypeStr will be ignored.
// In the case of duplicates between semantic conventions and custom resource attributes
// (e.g. container.id, datadog.container.tag.container_id) the semantic convention takes
// precedence.
func ContainerTagsFromResourceAttributes(attrs pcommon.Map) map[string]string {
	ddtags := make(map[string]string)
	attrs.Range(func(key string, value pcommon.Value) bool {
		// Semantic Conventions
		if datadogKey, found := ContainerMappings[key]; found && value.Str() != "" {
			ddtags[datadogKey] = value.Str()
		}
		// Custom (datadog.container.tag namespace)
		if strings.HasPrefix(key, customContainerTagPrefix) {
			customKey := strings.TrimPrefix(key, customContainerTagPrefix)
			if customKey != "" && value.Str() != "" {
				// Do not replace if set via semantic conventions mappings.
				if _, found := ddtags[customKey]; !found {
					ddtags[customKey] = value.Str()
				}
			}
		}
		return true
	})
	return ddtags
}

// ContainerTagFromAttributes extracts the value of _dd.tags.container from the given
// set of attributes.
// Deprecated: Deprecated in favor of ContainerTagFromResourceAttributes.
func ContainerTagFromAttributes(attr map[string]string) map[string]string {
	ddtags := make(map[string]string)
	for key, val := range attr {
		datadogKey, found := ContainerMappings[key]
		if !found {
			continue
		}
		ddtags[datadogKey] = val
	}
	return ddtags
}

// GetOTelAttrFromEitherMap returns the matched value as a string in either attribute map with the given keys.
// If there are multiple keys present, the first matched one is returned.
// If the key is present in both maps, map1 takes precedence.
// If normalize is true, normalize the return value with NormalizeTagValue.
func GetOTelAttrFromEitherMap(map1 pcommon.Map, map2 pcommon.Map, normalize bool, keys ...string) string {
	if val := GetOTelAttrVal(map1, normalize, keys...); val != "" {
		return val
	}
	return GetOTelAttrVal(map2, normalize, keys...)
}

// GetHostname returns the DD hostname based on OTel signal and resource attributes, with signal-level taking precedence.
func GetHostname(resattrs pcommon.Map, hostFromAttributesHandler HostFromAttributesHandler, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool, fallbackOverride *string) (string, bool) {
	foundHostNameFromOTelSemantics := true
	if useDatadogNamespaceIfPresent {
		host := GetOTelAttrVal(resattrs, true, DDNamespaceKeys.Host())
		if host != "" {
			return host, true
		}
	}
	if !ignoreMissingDatadogFields {
		// Try to get source from resource attributes using translator logic
		src, srcok := SourceFromAttrs(resattrs, hostFromAttributesHandler)
		if !srcok {
			foundHostNameFromOTelSemantics = false
			if v := GetOTelAttrVal(resattrs, false, "_dd.hostname"); v != "" {
				src = source.Source{Kind: source.HostnameKind, Identifier: v}
				srcok = true
			}
		}
		if srcok {
			switch src.Kind {
			case source.HostnameKind:
				return src.Identifier, foundHostNameFromOTelSemantics
			default:
				// We are not on a hostname (serverless), hence the hostname is empty
				return "", foundHostNameFromOTelSemantics
			}
		}
	}
	if fallbackOverride != nil {
		// fallback hostname from Agent conf.Hostname
		return *fallbackOverride, foundHostNameFromOTelSemantics
	}
	return "", foundHostNameFromOTelSemantics
}

// GetEnv returns the environment based on OTel resource attributes
func GetEnv(resattrs pcommon.Map, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool, fallbackOverride *string) (env string) {
	if useDatadogNamespaceIfPresent {
		env = GetOTelAttrVal(resattrs, true, DDNamespaceKeys.Env())
		if env != "" {
			return env
		}
	}
	if !ignoreMissingDatadogFields {
		env = GetOTelAttrVal(resattrs, true, APMConventionKeys.Env())
		if env != "" {
			return env
		}
		env = GetOTelAttrVal(resattrs, true, string(semconv1_27.DeploymentEnvironmentNameKey), string(semconv1_12.DeploymentEnvironmentKey))
		if env != "" {
			return env
		}
	}
	if fallbackOverride != nil {
		return *fallbackOverride
	}
	return DefaultOTLPEnvironmentName
}

// GetService returns the DD service name based on OTel resource attributes.
func GetService(resattrs pcommon.Map, normalize bool, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool, fallbackOverride *string) string {
	svc := ""
	if useDatadogNamespaceIfPresent {
		svc = GetOTelAttrVal(resattrs, false, DDNamespaceKeys.Service())
	}
	if svc == "" && !ignoreMissingDatadogFields {
		svc = GetOTelAttrVal(resattrs, false, APMConventionKeys.Service())
		if svc == "" {
			svc = GetOTelAttrVal(resattrs, false, string(semconv1_27.ServiceNameKey))
		}
	}
	if svc == "" {
		if fallbackOverride != nil {
			return *fallbackOverride
		} else {
			svc = DefaultOTLPServiceName
		}
	}
	if normalize {
		newsvc, err := normalizeutil.NormalizeService(svc, "")
		switch err {
		case normalizeutil.ErrTooLong:
			log.Debugf("Fixing malformed trace. Service is too long (reason:service_truncate), truncating span.service to length=%d: %s", normalizeutil.MaxServiceLen, svc)
		case normalizeutil.ErrInvalid:
			log.Debugf("Fixing malformed trace. Service is invalid (reason:service_invalid), replacing invalid span.service=%s with fallback span.service=%s", svc, newsvc)
		}
		svc = newsvc
	}
	return svc
}

// GetResourceName returns the DD resource name based on OTel span kind + signal attributes.
func GetResourceName(spanKind ptrace.SpanKind, signalattrs pcommon.Map, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool, fallbackOverride *string) (resName string) {
	defer func() {
		if len(resName) > normalizeutil.MaxResourceLen {
			resName = resName[:normalizeutil.MaxResourceLen]
		}
	}()
	if useDatadogNamespaceIfPresent {
		resName = GetOTelAttrVal(signalattrs, false, DDNamespaceKeys.ResourceName())
		if resName != "" {
			return resName
		}
	}
	if !ignoreMissingDatadogFields {
		if m := GetOTelAttrVal(signalattrs, false, APMConventionKeys.ResourceName()); m != "" {
			return m
		}

		// HTTP method + route logic
		if method := GetOTelAttrVal(signalattrs, false, string(semconv1_27.HTTPRequestMethodKey), string(semconv1_12.HTTPMethodKey)); method != "" {
			if method == "_OTHER" {
				method = "HTTP"
			}
			resName = method
			if spanKind == ptrace.SpanKindServer {
				if route := GetOTelAttrVal(signalattrs, false, string(semconv1_12.HTTPRouteKey)); route != "" {
					resName = resName + " " + route
				}
			}
			return
		}

		// Messaging operation logic
		if operation := GetOTelAttrVal(signalattrs, false, string(semconv1_12.MessagingOperationKey)); operation != "" {
			resName = operation
			if dest := GetOTelAttrVal(signalattrs, false, string(semconv1_12.MessagingDestinationKey), string(semconv1_17.MessagingDestinationNameKey)); dest != "" {
				resName = resName + " " + dest
			}
			return
		}

		// RPC method logic
		if method := GetOTelAttrVal(signalattrs, false, string(semconv1_12.RPCMethodKey)); method != "" {
			resName = method
			if svc := GetOTelAttrVal(signalattrs, false, string(semconv1_12.RPCServiceKey)); svc != "" {
				resName = resName + " " + svc
			}
			return
		}

		// GraphQL operation logic
		if opType := GetOTelAttrVal(signalattrs, false, string(semconv1_17.GraphqlOperationTypeKey)); opType != "" {
			resName = opType
			if name := GetOTelAttrVal(signalattrs, false, string(semconv1_17.GraphqlOperationNameKey)); name != "" {
				resName = resName + " " + name
			}
			return
		}

		// Database operation logic
		if dbSystem := GetOTelAttrVal(signalattrs, false, string(semconv1_12.DBSystemKey)); dbSystem != "" {
			if statement := GetOTelAttrVal(signalattrs, false, string(semconv1_12.DBStatementKey)); statement != "" {
				resName = statement
				return
			}
			if dbQuery := GetOTelAttrVal(signalattrs, false, string(semconv1_27.DBQueryTextKey)); dbQuery != "" {
				resName = dbQuery
				return
			}
		}
	}

	if fallbackOverride != nil {
		resName = *fallbackOverride
		return
	}
	return ""
}

// GetOperationName returns the DD operation name based on OTel span kind, signal and resource attributes, and given configs.
func GetOperationName(
	spanKind ptrace.SpanKind,
	sattr pcommon.Map,
	ignoreMissingDatadogFields bool,
	useDatadogNamespaceIfPresent bool,
	fallbackOverride *string,
) string {
	if useDatadogNamespaceIfPresent {
		name := GetOTelAttrVal(sattr, true, DDNamespaceKeys.OperationName())
		if name != "" {
			return name
		}
	}
	if !ignoreMissingDatadogFields {
		if operationName := GetOTelAttrVal(sattr, true, APMConventionKeys.OperationName()); operationName != "" {
			return operationName
		}

		isClient := spanKind == ptrace.SpanKindClient
		isServer := spanKind == ptrace.SpanKindServer

		// http
		if method := GetOTelAttrVal(sattr, true, string(semconv1_27.HTTPRequestMethodKey), string(semconv1_17.HTTPMethodKey)); method != "" {
			if isServer {
				return "http.server.request"
			}
			if isClient {
				return "http.client.request"
			}
		}

		// database
		if v := GetOTelAttrVal(sattr, true, string(semconv1_27.DBSystemKey)); v != "" && isClient {
			return v + ".query"
		}

		// messaging
		system := GetOTelAttrVal(sattr, true, string(semconv1_27.MessagingSystemKey))
		op := GetOTelAttrVal(sattr, true, string(semconv1_17.MessagingOperationKey))
		if system != "" && op != "" {
			switch spanKind {
			case ptrace.SpanKindClient, ptrace.SpanKindServer, ptrace.SpanKindConsumer, ptrace.SpanKindProducer:
				return system + "." + op
			}
		}

		// RPC & AWS
		rpcValue := GetOTelAttrVal(sattr, true, string(semconv1_27.RPCSystemKey))
		isRPC := rpcValue != ""
		isAws := isRPC && (rpcValue == "aws-api")
		// AWS client
		if isAws && isClient {
			if service := GetOTelAttrVal(sattr, true, string(semconv1_27.RPCServiceKey)); service != "" {
				return "aws." + service + ".request"
			}
			return "aws.client.request"
		}

		// RPC client
		if isRPC && isClient {
			return rpcValue + ".client.request"
		}
		// RPC server
		if isRPC && isServer {
			return rpcValue + ".server.request"
		}

		// FAAS client
		provider := GetOTelAttrVal(sattr, true, string(semconv1_27.FaaSInvokedProviderKey))
		invokedName := GetOTelAttrVal(sattr, true, string(semconv1_27.FaaSInvokedNameKey))
		if provider != "" && invokedName != "" && isClient {
			return provider + "." + invokedName + ".invoke"
		}

		// FAAS server
		trigger := GetOTelAttrVal(sattr, true, string(semconv1_27.FaaSTriggerKey))
		if trigger != "" && isServer {
			return trigger + ".invoke"
		}

		// GraphQL
		if GetOTelAttrVal(sattr, true, string(semconv1_27.GraphqlOperationTypeKey)) != "" {
			return "graphql.server.request"
		}

		// if nothing matches, checking for generic http server/client
		protocol := GetOTelAttrVal(sattr, true, string(semconv1_27.NetworkProtocolNameKey))
		if isServer {
			if protocol != "" {
				return protocol + ".server.request"
			}
			return "server.request"
		} else if isClient {
			if protocol != "" {
				return protocol + ".client.request"
			}
			return "client.request"
		}
	}

	if fallbackOverride != nil {
		return *fallbackOverride
	}
	if spanKind != ptrace.SpanKindUnspecified {
		return GetSpanKindName(spanKind)
	}
	return GetSpanKindName(ptrace.SpanKindInternal)
}

// GetSpanType returns the DD span type based on OTel span kind + signal attributes.
func GetSpanType(spanKind ptrace.SpanKind, signalattrs pcommon.Map, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool, fallbackOverride *string) string {
	var typ string
	if useDatadogNamespaceIfPresent {
		typ = GetOTelAttrVal(signalattrs, false, DDNamespaceKeys.SpanType())
		if typ != "" {
			return typ
		}
	}
	if !ignoreMissingDatadogFields {
		typ = GetOTelAttrVal(signalattrs, false, APMConventionKeys.SpanType())
		if typ != "" {
			return typ
		}

		switch spanKind {
		case ptrace.SpanKindServer:
			return "web"
		case ptrace.SpanKindClient:
			db := GetOTelAttrVal(signalattrs, false, string(semconv1_6_1.DBSystemKey))
			if db == "" {
				typ = "http"
			} else {
				typ = checkDBType(db)
			}
		}
		if typ != "" {
			return typ
		}
	}
	if fallbackOverride != nil {
		return *fallbackOverride
	}
	return "custom"
}

// Database span type constants (from agent)
const (
	spanTypeSQL           = "sql"
	spanTypeCassandra     = "cassandra"
	spanTypeRedis         = "redis"
	spanTypeMemcached     = "memcached"
	spanTypeMongoDB       = "mongodb"
	spanTypeElasticsearch = "elasticsearch"
	spanTypeOpenSearch    = "opensearch"
	spanTypeDB            = "db"
)

// dbTypes maps database systems to their corresponding span types (copied from agent)
var dbTypes = map[string]string{
	// SQL db types
	semconv1_12.DBSystemOtherSQL.Value.AsString():    spanTypeSQL,
	semconv1_12.DBSystemMSSQL.Value.AsString():       spanTypeSQL,
	semconv1_12.DBSystemMySQL.Value.AsString():       spanTypeSQL,
	semconv1_12.DBSystemOracle.Value.AsString():      spanTypeSQL,
	semconv1_12.DBSystemDB2.Value.AsString():         spanTypeSQL,
	semconv1_12.DBSystemPostgreSQL.Value.AsString():  spanTypeSQL,
	semconv1_12.DBSystemRedshift.Value.AsString():    spanTypeSQL,
	semconv1_12.DBSystemCloudscape.Value.AsString():  spanTypeSQL,
	semconv1_12.DBSystemHSQLDB.Value.AsString():      spanTypeSQL,
	semconv1_12.DBSystemMaxDB.Value.AsString():       spanTypeSQL,
	semconv1_12.DBSystemIngres.Value.AsString():      spanTypeSQL,
	semconv1_12.DBSystemFirstSQL.Value.AsString():    spanTypeSQL,
	semconv1_12.DBSystemEDB.Value.AsString():         spanTypeSQL,
	semconv1_12.DBSystemCache.Value.AsString():       spanTypeSQL,
	semconv1_12.DBSystemFirebird.Value.AsString():    spanTypeSQL,
	semconv1_12.DBSystemDerby.Value.AsString():       spanTypeSQL,
	semconv1_12.DBSystemInformix.Value.AsString():    spanTypeSQL,
	semconv1_12.DBSystemMariaDB.Value.AsString():     spanTypeSQL,
	semconv1_12.DBSystemSqlite.Value.AsString():      spanTypeSQL,
	semconv1_12.DBSystemSybase.Value.AsString():      spanTypeSQL,
	semconv1_12.DBSystemTeradata.Value.AsString():    spanTypeSQL,
	semconv1_12.DBSystemVertica.Value.AsString():     spanTypeSQL,
	semconv1_12.DBSystemH2.Value.AsString():          spanTypeSQL,
	semconv1_12.DBSystemColdfusion.Value.AsString():  spanTypeSQL,
	semconv1_12.DBSystemCockroachdb.Value.AsString(): spanTypeSQL,
	semconv1_12.DBSystemProgress.Value.AsString():    spanTypeSQL,
	semconv1_12.DBSystemHanaDB.Value.AsString():      spanTypeSQL,
	semconv1_12.DBSystemAdabas.Value.AsString():      spanTypeSQL,
	semconv1_12.DBSystemFilemaker.Value.AsString():   spanTypeSQL,
	semconv1_12.DBSystemInstantDB.Value.AsString():   spanTypeSQL,
	semconv1_12.DBSystemInterbase.Value.AsString():   spanTypeSQL,
	semconv1_12.DBSystemNetezza.Value.AsString():     spanTypeSQL,
	semconv1_12.DBSystemPervasive.Value.AsString():   spanTypeSQL,
	semconv1_12.DBSystemPointbase.Value.AsString():   spanTypeSQL,
	semconv1_17.DBSystemClickhouse.Value.AsString():  spanTypeSQL, // not in semconv 1.6.1

	// Cassandra db types
	semconv1_12.DBSystemCassandra.Value.AsString(): spanTypeCassandra,

	// Redis db types
	semconv1_12.DBSystemRedis.Value.AsString(): spanTypeRedis,

	// Memcached db types
	semconv1_12.DBSystemMemcached.Value.AsString(): spanTypeMemcached,

	// MongoDB db types
	semconv1_12.DBSystemMongoDB.Value.AsString(): spanTypeMongoDB,

	// Elasticsearch db types
	semconv1_12.DBSystemElasticsearch.Value.AsString(): spanTypeElasticsearch,

	// OpenSearch db types, not in semconv1_12 1.6.1
	semconv1_17.DBSystemOpensearch.Value.AsString(): spanTypeOpenSearch,

	// Generic db types
	semconv1_12.DBSystemHive.Value.AsString():      spanTypeDB,
	semconv1_12.DBSystemHBase.Value.AsString():     spanTypeDB,
	semconv1_12.DBSystemNeo4j.Value.AsString():     spanTypeDB,
	semconv1_12.DBSystemCouchbase.Value.AsString(): spanTypeDB,
	semconv1_12.DBSystemCouchDB.Value.AsString():   spanTypeDB,
	semconv1_12.DBSystemCosmosDB.Value.AsString():  spanTypeDB,
	semconv1_12.DBSystemDynamoDB.Value.AsString():  spanTypeDB,
	semconv1_12.DBSystemGeode.Value.AsString():     spanTypeDB,
}

// checkDBType checks if the dbType is a known db type and returns the corresponding span.Type (from agent)
func checkDBType(dbType string) string {
	spanType, ok := dbTypes[dbType]
	if ok {
		return spanType
	}
	// span type not found, return generic db type
	return spanTypeDB
}

// GetVersion returns the version based on OTel resource attributes.
func GetVersion(resattrs pcommon.Map, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool, fallbackOverride *string) string {
	if useDatadogNamespaceIfPresent {
		version := GetOTelAttrVal(resattrs, true, DDNamespaceKeys.Version())
		if version != "" {
			return version
		}
	}
	if !ignoreMissingDatadogFields {
		version := GetOTelAttrVal(resattrs, true, APMConventionKeys.Version())
		if version != "" {
			return version
		}
		version = GetOTelAttrVal(resattrs, true, string(semconv1_27.ServiceVersionKey))
		if version != "" {
			return version
		}
	}
	if fallbackOverride != nil {
		return *fallbackOverride
	}
	return ""
}

// GetStatusCode returns the HTTP status code based on OTel signal and resource attributes, with signal-level taking precedence.
func GetStatusCode(signalattrs pcommon.Map, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool, fallbackOverride *uint32) (uint32, error) {
	ret := getStatusCodeCandidate(signalattrs, ignoreMissingDatadogFields, useDatadogNamespaceIfPresent, fallbackOverride)
	switch ret.Type() {
	case pcommon.ValueTypeInt:
		return uint32(ret.Int()), nil
	case pcommon.ValueTypeDouble:
		return uint32(ret.Int()), nil
	case pcommon.ValueTypeStr:
		if code, err := strconv.ParseUint(ret.AsString(), 10, 32); err == nil {
			return uint32(code), nil
		} else {
			return 0, fmt.Errorf("invalid status code %s", ret.AsString())
		}
	default:
		return 0, fmt.Errorf("unsupported type %s", ret.Type())
	}
}

func getStatusCodeCandidate(signalattrs pcommon.Map, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool, fallbackOverride *uint32) pcommon.Value {
	if useDatadogNamespaceIfPresent {
		if code, ok := signalattrs.Get(DDNamespaceKeys.HTTPStatusCode()); ok {
			return code
		}
	}
	if !ignoreMissingDatadogFields {
		if code, ok := signalattrs.Get(APMConventionKeys.HTTPStatusCode()); ok {
			return code
		}
		if code, ok := signalattrs.Get(string(semconv1_17.HTTPStatusCodeKey)); ok {
			return code
		}
		if code, ok := signalattrs.Get(string(semconv1_27.HTTPResponseStatusCodeKey)); ok {
			return code
		}
	}
	if fallbackOverride != nil {
		return pcommon.NewValueInt(int64(*fallbackOverride))
	}
	return pcommon.NewValueInt(0)
}

// GetContainerID returns the container ID based on OTel span and resource attributes, with span taking precedence.
func GetContainerID(resourceattrs pcommon.Map, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool, fallbackOverride *string) string {
	if useDatadogNamespaceIfPresent {
		cid := GetOTelAttrVal(resourceattrs, true, DDNamespaceKeys.ContainerID())
		if cid != "" {
			return cid
		}
	}
	if !ignoreMissingDatadogFields {
		cid := GetOTelAttrVal(resourceattrs, true, APMConventionKeys.ContainerID())
		if cid != "" {
			return cid
		}
		cid = GetOTelAttrVal(resourceattrs, true, string(semconv1_27.ContainerIDKey))
		if cid != "" {
			return cid
		}
	}
	if fallbackOverride != nil {
		return *fallbackOverride
	}
	return ""
}

func GetSpanKind(spanKind ptrace.SpanKind, sattr pcommon.Map, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool, fallbackOverride *string) string {
	if useDatadogNamespaceIfPresent {
		incomingSpanKindName := GetOTelAttrVal(sattr, true, DDNamespaceKeys.SpanKind())
		if incomingSpanKindName != "" {
			return incomingSpanKindName
		}
	}
	if !ignoreMissingDatadogFields {
		if kind := GetOTelAttrVal(sattr, true, APMConventionKeys.SpanKind()); kind != "" {
			return kind
		}
		return GetSpanKindName(spanKind)
	}
	if fallbackOverride != nil {
		return *fallbackOverride
	}
	return "unspecified"
}

var spanKindNames = map[ptrace.SpanKind]string{
	ptrace.SpanKindUnspecified: "unspecified",
	ptrace.SpanKindInternal:    "internal",
	ptrace.SpanKindServer:      "server",
	ptrace.SpanKindClient:      "client",
	ptrace.SpanKindProducer:    "producer",
	ptrace.SpanKindConsumer:    "consumer",
}

// OTelSpanKindName converts the given SpanKind to a valid Datadog span kind name.
func GetSpanKindName(k ptrace.SpanKind) string {
	name, ok := spanKindNames[k]
	if !ok {
		return "unspecified"
	}
	return name
}

// GetContainerTags returns a list of DD container tags in an OTel map's attributes.
// Tags are always normalized.
func GetContainerTags(rattrs pcommon.Map, tagKeys []string) []string {
	ddtags := make(map[string]string)
	semConvsCB := func(key string, value pcommon.Value) bool {
		// Semantic Conventions
		if datadogKey, found := ContainerMappings[key]; found && value.Str() != "" {
			ddtags[datadogKey] = value.Str()
		}
		ddtags[key] = value.Str()
		return true
	}
	rattrs.Range(semConvsCB)

	ddNamespaceCB := func(key string, value pcommon.Value) bool {
		// Custom (datadog.container.tag namespace)
		if strings.HasPrefix(key, customContainerTagPrefix) {
			customKey := strings.TrimPrefix(key, customContainerTagPrefix)
			if customKey != "" && value.Str() != "" {
				// Do not replace if set via semantic conventions mappings.
				if _, found := ddtags[customKey]; !found {
					ddtags[customKey] = value.Str()
				}
			}
		}
		return true
	}
	rattrs.Range(ddNamespaceCB)

	var containerTags []string
	for _, key := range tagKeys {
		outputKey := ""
		outputValue := ""
		keyToUse := key
		if mappedKey, ok := ContainerMappings[key]; ok {
			keyToUse = mappedKey
		}
		// Otherwise populate as additional container tags
		if val, ok := ddtags[keyToUse]; ok && val != "" {
			outputKey = keyToUse
			outputValue = val
			t := normalizeutil.NormalizeTag(outputKey + ":" + outputValue)
			containerTags = append(containerTags, t)
		}
	}
	return containerTags
}

// GetOTelAttrVal returns the matched value as a string in the input map with the given keys.
// If there are multiple keys present, the first matched one is returned.
// If normalize is true, normalize the return value with NormalizeTagValue.
func GetOTelAttrVal(attrs pcommon.Map, normalize bool, keys ...string) string {
	val := ""
	for _, key := range keys {
		attrval, exists := attrs.Get(key)
		if exists {
			val = attrval.AsString()
			break
		}
	}

	if normalize {
		val = normalizeutil.NormalizeTagValue(val)
	}

	return val
}

// normalizeTag normalizes a tag (simplified version of agent's normalizeutil.NormalizeTag)
func normalizeTag(tag string) string {
	// Basic validation - must contain colon
	if !strings.Contains(tag, ":") {
		return tag
	}

	// Replace invalid characters with underscores
	invalidChars := regexp.MustCompile(`[^\w.:/\-]`)
	tag = invalidChars.ReplaceAllString(tag, "_")

	// Trim and lowercase
	tag = strings.ToLower(strings.TrimSpace(tag))

	return tag
}

// GetSpecifiedKeysFromOTelAttributes returns a subset of OTel signal and resource attributes, with signal-level taking precedence.
// e.g. Useful for extracting peer tags
func GetSpecifiedKeysFromOTelAttributes(signalattrs pcommon.Map, resattrs pcommon.Map, peerTagKeys map[string]struct{}) []string {
	if peerTagKeys == nil {
		return []string{}
	}
	var peerTagsMap map[string]string = make(map[string]string, len(peerTagKeys))

	cb := func(k string, v pcommon.Value) bool {
		val := v.AsString()
		if _, ok := peerTagKeys[k]; ok {
			peerTagsMap[k] = val
		}
		return true
	}

	// Signal overwrites res
	resattrs.Range(cb)
	signalattrs.Range(cb)

	peerTags := make([]string, 0, len(peerTagsMap))
	for k, v := range peerTagsMap {
		t := normalizeTag(k + ":" + v)
		peerTags = append(peerTags, t)
	}
	return peerTags
}
