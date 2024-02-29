// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var allowedEnvvarNames = []string{
	// Docker client
	"DOCKER_API_VERSION",
	"DOCKER_CONFIG",
	"DOCKER_CERT_PATH",
	"DOCKER_HOST",
	"DOCKER_TLS_VERIFY",

	// HOST vars used in containerized agent
	"HOST_ETC",
	"HOST_PROC",
	"HOST_ROOT",

	// Proxy settings
	"HTTP_PROXY",
	"HTTPS_PROXY",
	"NO_PROXY",
	"DD_PROXY_HTTP",
	"DD_PROXY_HTTPS",
	"DD_PROXY_NO_PROXY",

	// Go runtime
	"GOGC",
	"GODEBUG",
	"GOMAXPROCS",
	"GOTRACEBACK",

	// Trace agent
	"DD_APM_ENABLED",
	"DD_APM_NON_LOCAL_TRAFFIC",
	"DD_RECEIVER_PORT",   // deprecated
	"DD_IGNORE_RESOURCE", // deprecated
	"DD_APM_DD_URL",
	"DD_APM_ANALYZED_SPANS",
	"DD_CONNECTION_LIMIT", // deprecated
	"DD_APM_CONNECTION_LIMIT",
	"DD_APM_ENV",
	"DD_APM_RECEIVER_PORT",
	"DD_APM_IGNORE_RESOURCES",
	"DD_MAX_EPS ", // deprecated
	"DD_APM_MAX_EPS",
	"DD_APM_TPS", //deprecated
	"DD_APM_MAX_TPS",
	"DD_APM_ERROR_TPS",
	"DD_APM_ENABLE_RARE_SAMPLER",
	"DD_APM_DISABLE_RARE_SAMPLER", // deprecated
	"DD_APM_MAX_REMOTE_TPS",
	"DD_APM_MAX_MEMORY",
	"DD_APM_MAX_CPU_PERCENT",
	"DD_APM_FEATURES",
	"DD_APM_RECEIVER_SOCKET",
	"DD_APM_REPLACE_TAGS",
	"DD_APM_PROFILING_DD_URL",
	"DD_APM_WINDOWS_PIPE_BUFFER_SIZE",
	"DD_APM_REMOTE_TAGGER",
	"DD_APM_COMPUTE_STATS_BY_SPAN_KIND",
	"DD_APM_PEER_TAGS_AGGREGATION",
	"DD_APM_PEER_TAGS",
	"DD_APM_MAX_CATALOG_SERVICES",
	"DD_APM_RECEIVER_TIMEOUT",
	"DD_APM_MAX_PAYLOAD_SIZE",
	"DD_APM_LOG_FILE",
	"DD_APM_CONNECTION_RESET_INTERVAL",
	"DD_APM_WINDOWS_PIPE_NAME",
	"DD_APM_SYNC_FLUSHING",
	"DD_APM_FILTER_TAGS_REQUIRE",
	"DD_APM_FILTER_TAGS_REJECT",
	"DD_APM_INTERNAL_PROFILING_ENABLED",
	"DD_APM_DEBUGGER_DD_URL",
	"DD_APM_SYMDB_DD_URL",
	"DD_APM_OBFUSCATION_CREDIT_CARDS_ENABLED",
	"DD_APM_OBFUSCATION_CREDIT_CARDS_LUHN",
	"DD_APM_OBFUSCATION_ELASTICSEARCH_ENABLED",
	"DD_APM_OBFUSCATION_ELASTICSEARCH_KEEP_VALUES",
	"DD_APM_OBFUSCATION_ELASTICSEARCH_OBFUSCATE_SQL_VALUES",
	"DD_APM_OBFUSCATION_HTTP_REMOVE_QUERY_STRING",
	"DD_APM_OBFUSCATION_HTTP_REMOVE_PATHS_WITH_DIGITS",
	"DD_APM_OBFUSCATION_MEMCACHED_ENABLED",
	"DD_APM_OBFUSCATION_MEMCACHED_KEEP_COMMAND",
	"DD_APM_OBFUSCATION_MONGODB_ENABLED",
	"DD_APM_OBFUSCATION_MONGODB_KEEP_VALUES",
	"DD_APM_OBFUSCATION_MONGODB_OBFUSCATE_SQL_VALUES",
	"DD_APM_OBFUSCATION_REDIS_ENABLED",
	"DD_APM_OBFUSCATION_REDIS_REMOVE_ALL_ARGS",
	"DD_APM_OBFUSCATION_REMOVE_STACK_TRACES",
	"DD_APM_OBFUSCATION_SQL_EXEC_PLAN_ENABLED",
	"DD_APM_OBFUSCATION_SQL_EXEC_PLAN_KEEP_VALUES",
	"DD_APM_OBFUSCATION_SQL_EXEC_PLAN_OBFUSCATE_SQL_VALUES",
	"DD_APM_OBFUSCATION_SQL_EXEC_PLAN_NORMALIZE_ENABLED",
	"DD_APM_OBFUSCATION_SQL_EXEC_PLAN_NORMALIZE_KEEP_VALUES",
	"DD_APM_OBFUSCATION_SQL_EXEC_PLAN_NORMALIZE_OBFUSCATE_SQL_VALUES",
	"DD_APM_DEBUG_PORT",
	"DD_APM_INSTRUMENTATION_ENABLED",
	"DD_APM_INSTRUMENTATION_ENABLED_NAMESPACES",
	"DD_APM_INSTRUMENTATION_DISABLED_NAMESPACES",

	// Process agent
	"DD_PROCESS_AGENT_URL",
	"DD_SCRUB_ARGS",
	"DD_CUSTOM_SENSITIVE_WORDS",
	"DD_STRIP_PROCESS_ARGS",
	"DD_LOGS_STDOUT",
	"LOG_LEVEL",
	"LOG_TO_CONSOLE",
	"DD_COLLECT_DOCKER_NETWORK",
	"DD_CONTAINER_BLACKLIST",
	"DD_CONTAINER_WHITELIST",
	"DD_CONTAINER_CACHE_DURATION",
	"DD_SYSTEM_PROBE_ENABLED",
	"DD_SYSTEM_PROBE_NETWORK_ENABLED",

	// CI
	"DD_INSIDE_CI",

	// Cluster agent
	"CHART_RELEASE_NAME",
	"AGENT_DAEMONSET",
	"CLUSTER_AGENT_DEPLOYMENT",
}

func getAllowedEnvvars() []string {
	allowed := allowedEnvvarNames
	allowed = append(allowed, config.Datadog.GetEnvVars()...)
	var found []string
	for _, envvar := range os.Environ() {
		parts := strings.SplitN(envvar, "=", 2)
		key := strings.ToUpper(parts[0])
		if strings.Contains(key, "_KEY") || strings.Contains(key, "_AUTH_TOKEN") {
			// `_key`-suffixed env vars are sensitive: don't track them
			// `_auth_token`-suffixed env vars are sensitive: don't track them
			continue
		}
		for _, envName := range allowed {
			if key == envName {
				found = append(found, envvar)
				continue
			}
		}
	}
	return found
}

// getEnvVars collects allowed envvars that can affect the agent's
// behaviour while not being handled by viper, in addition to the envvars handled by viper
func getEnvVars() ([]byte, error) {
	envvars := getAllowedEnvvars()

	var b bytes.Buffer
	if len(envvars) > 0 {
		fmt.Fprintln(&b, "Found the following envvars:")
		for _, envvar := range envvars {
			fmt.Fprintln(&b, " - ", envvar)
		}
	} else {
		fmt.Fprintln(&b, "Found no allowed envvar")
	}

	return b.Bytes(), nil
}
