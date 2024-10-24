// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Traces specifies the data type used for Vector override. See https://vector.dev/docs/reference/configuration/sources/datadog_agent/ for additional details.
const Traces DataType = "traces"

func setupAPM(config pkgconfigmodel.Setup) {
	config.BindEnv("apm_config.obfuscation.elasticsearch.enabled", "DD_APM_OBFUSCATION_ELASTICSEARCH_ENABLED")
	config.BindEnv("apm_config.obfuscation.elasticsearch.keep_values", "DD_APM_OBFUSCATION_ELASTICSEARCH_KEEP_VALUES")
	config.BindEnv("apm_config.obfuscation.elasticsearch.obfuscate_sql_values", "DD_APM_OBFUSCATION_ELASTICSEARCH_OBFUSCATE_SQL_VALUES")
	config.BindEnv("apm_config.obfuscation.opensearch.enabled", "DD_APM_OBFUSCATION_OPENSEARCH_ENABLED")
	config.BindEnv("apm_config.obfuscation.opensearch.keep_values", "DD_APM_OBFUSCATION_OPENSEARCH_KEEP_VALUES")
	config.BindEnv("apm_config.obfuscation.opensearch.obfuscate_sql_values", "DD_APM_OBFUSCATION_OPENSEARCH_OBFUSCATE_SQL_VALUES")
	config.BindEnv("apm_config.obfuscation.mongodb.enabled", "DD_APM_OBFUSCATION_MONGODB_ENABLED")
	config.BindEnv("apm_config.obfuscation.mongodb.keep_values", "DD_APM_OBFUSCATION_MONGODB_KEEP_VALUES")
	config.BindEnv("apm_config.obfuscation.mongodb.obfuscate_sql_values", "DD_APM_OBFUSCATION_MONGODB_OBFUSCATE_SQL_VALUES")
	config.BindEnv("apm_config.obfuscation.sql_exec_plan.enabled", "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_ENABLED")
	config.BindEnv("apm_config.obfuscation.sql_exec_plan.keep_values", "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_KEEP_VALUES")
	config.BindEnv("apm_config.obfuscation.sql_exec_plan.obfuscate_sql_values", "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_OBFUSCATE_SQL_VALUES")
	config.BindEnv("apm_config.obfuscation.sql_exec_plan_normalize.enabled", "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_NORMALIZE_ENABLED")
	config.BindEnv("apm_config.obfuscation.sql_exec_plan_normalize.keep_values", "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_NORMALIZE_KEEP_VALUES")
	config.BindEnv("apm_config.obfuscation.sql_exec_plan_normalize.obfuscate_sql_values", "DD_APM_OBFUSCATION_SQL_EXEC_PLAN_NORMALIZE_OBFUSCATE_SQL_VALUES")
	config.BindEnv("apm_config.obfuscation.http.remove_query_string", "DD_APM_OBFUSCATION_HTTP_REMOVE_QUERY_STRING")
	config.BindEnv("apm_config.obfuscation.http.remove_paths_with_digits", "DD_APM_OBFUSCATION_HTTP_REMOVE_PATHS_WITH_DIGITS")
	config.BindEnv("apm_config.obfuscation.remove_stack_traces", "DD_APM_OBFUSCATION_REMOVE_STACK_TRACES")
	config.BindEnv("apm_config.obfuscation.redis.enabled", "DD_APM_OBFUSCATION_REDIS_ENABLED")
	config.BindEnv("apm_config.obfuscation.redis.remove_all_args", "DD_APM_OBFUSCATION_REDIS_REMOVE_ALL_ARGS")
	config.BindEnv("apm_config.obfuscation.memcached.enabled", "DD_APM_OBFUSCATION_MEMCACHED_ENABLED")
	config.BindEnv("apm_config.obfuscation.memcached.keep_command", "DD_APM_OBFUSCATION_MEMCACHED_KEEP_COMMAND")
	config.SetKnown("apm_config.filter_tags.require")
	config.SetKnown("apm_config.filter_tags.reject")
	config.SetKnown("apm_config.filter_tags_regex.require")
	config.SetKnown("apm_config.filter_tags_regex.reject")
	config.SetKnown("apm_config.extra_sample_rate")
	config.SetKnown("apm_config.dd_agent_bin")
	config.SetKnown("apm_config.trace_writer.connection_limit")
	config.SetKnown("apm_config.trace_writer.queue_size")
	config.SetKnown("apm_config.service_writer.connection_limit")
	config.SetKnown("apm_config.service_writer.queue_size")
	config.SetKnown("apm_config.stats_writer.connection_limit")
	config.SetKnown("apm_config.stats_writer.queue_size")
	config.SetKnown("apm_config.analyzed_rate_by_service.*")
	config.SetKnown("apm_config.bucket_size_seconds")
	config.SetKnown("apm_config.watchdog_check_delay")
	config.SetKnown("apm_config.sync_flushing")
	config.SetKnown("apm_config.features")
	config.SetKnown("apm_config.max_catalog_entries")

	bindVectorOptions(config, Traces)

	if runtime.GOARCH == "386" && runtime.GOOS == "windows" {
		// on Windows-32 bit, the trace agent isn't installed.  Set the default to disabled
		// so that there aren't messages in the log about failing to start.
		config.BindEnvAndSetDefault("apm_config.enabled", false, "DD_APM_ENABLED")
	} else {
		config.BindEnvAndSetDefault("apm_config.enabled", true, "DD_APM_ENABLED")
	}

	config.BindEnvAndSetDefault("apm_config.receiver_enabled", true, "DD_APM_RECEIVER_ENABLED")
	config.BindEnvAndSetDefault("apm_config.receiver_port", 8126, "DD_APM_RECEIVER_PORT", "DD_RECEIVER_PORT")
	config.BindEnvAndSetDefault("apm_config.windows_pipe_buffer_size", 1_000_000, "DD_APM_WINDOWS_PIPE_BUFFER_SIZE")                          //nolint:errcheck
	config.BindEnvAndSetDefault("apm_config.windows_pipe_security_descriptor", "D:AI(A;;GA;;;WD)", "DD_APM_WINDOWS_PIPE_SECURITY_DESCRIPTOR") //nolint:errcheck
	config.BindEnvAndSetDefault("apm_config.remote_tagger", true, "DD_APM_REMOTE_TAGGER")                                                     //nolint:errcheck
	config.BindEnvAndSetDefault("apm_config.peer_service_aggregation", true, "DD_APM_PEER_SERVICE_AGGREGATION")                               //nolint:errcheck
	config.BindEnvAndSetDefault("apm_config.peer_tags_aggregation", true, "DD_APM_PEER_TAGS_AGGREGATION")                                     //nolint:errcheck
	config.BindEnvAndSetDefault("apm_config.compute_stats_by_span_kind", true, "DD_APM_COMPUTE_STATS_BY_SPAN_KIND")                           //nolint:errcheck
	config.BindEnvAndSetDefault("apm_config.instrumentation.enabled", false, "DD_APM_INSTRUMENTATION_ENABLED")
	config.BindEnvAndSetDefault("apm_config.instrumentation.enabled_namespaces", []string{}, "DD_APM_INSTRUMENTATION_ENABLED_NAMESPACES")
	config.BindEnvAndSetDefault("apm_config.instrumentation.disabled_namespaces", []string{}, "DD_APM_INSTRUMENTATION_DISABLED_NAMESPACES")
	config.BindEnvAndSetDefault("apm_config.instrumentation.lib_versions", map[string]string{}, "DD_APM_INSTRUMENTATION_LIB_VERSIONS")
	// Note(stanistan): The flag "DD_APM_INSTRUMENTATION_VERSION"
	//                  will remain undocumented for the duration of the beta.
	//                  We intend to only switch back to v1 if beta customers have issues
	//                  to troubleshoot. v1 will be dropped (and possibly the flag)
	//                  in 7.58 or 7.59 (this is going out in 7.57).
	config.BindEnvAndSetDefault("apm_config.instrumentation.version", "v2", "DD_APM_INSTRUMENTATION_VERSION")
	// Default Image Tag for the APM Inject package (https://hub.docker.com/r/datadog/apm-inject/tags).
	// We pin to a major version by default.
	config.BindEnvAndSetDefault("apm_config.instrumentation.injector_image_tag", "0", "DD_APM_INSTRUMENTATION_INJECTOR_IMAGE_TAG")

	config.BindEnv("apm_config.max_catalog_services", "DD_APM_MAX_CATALOG_SERVICES")
	config.BindEnv("apm_config.receiver_timeout", "DD_APM_RECEIVER_TIMEOUT")
	config.BindEnv("apm_config.max_payload_size", "DD_APM_MAX_PAYLOAD_SIZE")
	config.BindEnv("apm_config.trace_buffer", "DD_APM_TRACE_BUFFER")
	config.BindEnv("apm_config.decoders", "DD_APM_DECODERS")
	config.BindEnv("apm_config.max_connections", "DD_APM_MAX_CONNECTIONS")
	config.BindEnv("apm_config.decoder_timeout", "DD_APM_DECODER_TIMEOUT")
	config.BindEnv("apm_config.log_file", "DD_APM_LOG_FILE")
	config.BindEnv("apm_config.max_events_per_second", "DD_APM_MAX_EPS", "DD_MAX_EPS")
	config.BindEnv("apm_config.max_traces_per_second", "DD_APM_MAX_TPS", "DD_MAX_TPS") // deprecated
	config.BindEnv("apm_config.target_traces_per_second", "DD_APM_TARGET_TPS")
	config.BindEnv("apm_config.errors_per_second", "DD_APM_ERROR_TPS")
	config.BindEnv("apm_config.enable_rare_sampler", "DD_APM_ENABLE_RARE_SAMPLER")
	config.BindEnv("apm_config.disable_rare_sampler", "DD_APM_DISABLE_RARE_SAMPLER") // Deprecated
	config.BindEnv("apm_config.max_remote_traces_per_second", "DD_APM_MAX_REMOTE_TPS")
	config.BindEnv("apm_config.probabilistic_sampler.enabled", "DD_APM_PROBABILISTIC_SAMPLER_ENABLED")
	config.BindEnv("apm_config.probabilistic_sampler.sampling_percentage", "DD_APM_PROBABILISTIC_SAMPLER_SAMPLING_PERCENTAGE")
	config.BindEnv("apm_config.probabilistic_sampler.hash_seed", "DD_APM_PROBABILISTIC_SAMPLER_HASH_SEED")

	config.BindEnv("apm_config.max_memory", "DD_APM_MAX_MEMORY")
	config.BindEnv("apm_config.max_cpu_percent", "DD_APM_MAX_CPU_PERCENT")
	config.BindEnv("apm_config.env", "DD_APM_ENV")
	config.BindEnv("apm_config.apm_non_local_traffic", "DD_APM_NON_LOCAL_TRAFFIC")
	config.BindEnv("apm_config.apm_dd_url", "DD_APM_DD_URL")
	config.BindEnv("apm_config.connection_limit", "DD_APM_CONNECTION_LIMIT", "DD_CONNECTION_LIMIT")
	config.BindEnv("apm_config.connection_reset_interval", "DD_APM_CONNECTION_RESET_INTERVAL")
	config.BindEnv("apm_config.max_sender_retries", "DD_APM_MAX_SENDER_RETRIES")
	config.BindEnv("apm_config.profiling_dd_url", "DD_APM_PROFILING_DD_URL")
	config.BindEnv("apm_config.profiling_additional_endpoints", "DD_APM_PROFILING_ADDITIONAL_ENDPOINTS")
	config.BindEnv("apm_config.additional_endpoints", "DD_APM_ADDITIONAL_ENDPOINTS")
	config.BindEnv("apm_config.replace_tags", "DD_APM_REPLACE_TAGS")
	config.BindEnv("apm_config.analyzed_spans", "DD_APM_ANALYZED_SPANS")
	config.BindEnv("apm_config.ignore_resources", "DD_APM_IGNORE_RESOURCES", "DD_IGNORE_RESOURCE")
	config.BindEnv("apm_config.receiver_socket", "DD_APM_RECEIVER_SOCKET")
	config.BindEnv("apm_config.windows_pipe_name", "DD_APM_WINDOWS_PIPE_NAME")
	config.BindEnv("apm_config.sync_flushing", "DD_APM_SYNC_FLUSHING")
	config.BindEnv("apm_config.filter_tags.require", "DD_APM_FILTER_TAGS_REQUIRE")
	config.BindEnv("apm_config.filter_tags.reject", "DD_APM_FILTER_TAGS_REJECT")
	config.BindEnv("apm_config.filter_tags_regex.reject", "DD_APM_FILTER_TAGS_REGEX_REJECT")
	config.BindEnv("apm_config.filter_tags_regex.require", "DD_APM_FILTER_TAGS_REGEX_REQUIRE")
	config.BindEnv("apm_config.internal_profiling.enabled", "DD_APM_INTERNAL_PROFILING_ENABLED")
	config.BindEnv("apm_config.debugger_dd_url", "DD_APM_DEBUGGER_DD_URL")
	config.BindEnv("apm_config.debugger_api_key", "DD_APM_DEBUGGER_API_KEY")
	config.BindEnv("apm_config.debugger_additional_endpoints", "DD_APM_DEBUGGER_ADDITIONAL_ENDPOINTS")
	config.BindEnv("apm_config.debugger_diagnostics_dd_url", "DD_APM_DEBUGGER_DIAGNOSTICS_DD_URL")
	config.BindEnv("apm_config.debugger_diagnostics_api_key", "DD_APM_DEBUGGER_DIAGNOSTICS_API_KEY")
	config.BindEnv("apm_config.debugger_diagnostics_additional_endpoints", "DD_APM_DEBUGGER_DIAGNOSTICS_ADDITIONAL_ENDPOINTS")
	config.BindEnv("apm_config.symdb_dd_url", "DD_APM_SYMDB_DD_URL")
	config.BindEnv("apm_config.symdb_api_key", "DD_APM_SYMDB_API_KEY")
	config.BindEnv("apm_config.symdb_additional_endpoints", "DD_APM_SYMDB_ADDITIONAL_ENDPOINTS")
	config.BindEnvAndSetDefault("apm_config.telemetry.enabled", true, "DD_APM_TELEMETRY_ENABLED")
	config.BindEnv("apm_config.telemetry.dd_url", "DD_APM_TELEMETRY_DD_URL")
	config.BindEnv("apm_config.telemetry.additional_endpoints", "DD_APM_TELEMETRY_ADDITIONAL_ENDPOINTS")
	config.BindEnv("apm_config.install_id", "DD_INSTRUMENTATION_INSTALL_ID")
	config.BindEnv("apm_config.install_type", "DD_INSTRUMENTATION_INSTALL_TYPE")
	config.BindEnv("apm_config.install_time", "DD_INSTRUMENTATION_INSTALL_TIME")
	config.BindEnv("apm_config.obfuscation.credit_cards.enabled", "DD_APM_OBFUSCATION_CREDIT_CARDS_ENABLED")
	config.BindEnv("apm_config.obfuscation.credit_cards.luhn", "DD_APM_OBFUSCATION_CREDIT_CARDS_LUHN")
	config.BindEnvAndSetDefault("apm_config.debug.port", 5012, "DD_APM_DEBUG_PORT")
	config.BindEnv("apm_config.features", "DD_APM_FEATURES")
	config.ParseEnvAsStringSlice("apm_config.features", func(s string) []string {
		// Either commas or spaces can be used as separators.
		// Comma takes precedence as it was the only supported separator in the past.
		// Mixing separators is not supported.
		var res []string
		if strings.ContainsRune(s, ',') {
			res = strings.Split(s, ",")
		} else {
			res = strings.Split(s, " ")
		}
		for i, v := range res {
			res[i] = strings.TrimSpace(v)
		}
		return res
	})

	config.ParseEnvAsStringSlice("apm_config.ignore_resources", func(in string) []string {
		r, err := splitCSVString(in, ',')
		if err != nil {
			log.Warnf(`"apm_config.ignore_resources" can not be parsed: %v`, err)
			return []string{}
		}
		return r
	})

	config.ParseEnvAsStringSlice("apm_config.filter_tags.require", parseKVList("apm_config.filter_tags.require"))
	config.ParseEnvAsStringSlice("apm_config.filter_tags.reject", parseKVList("apm_config.filter_tags.reject"))
	config.ParseEnvAsStringSlice("apm_config.filter_tags_regex.require", parseKVList("apm_config.filter_tags_regex.require"))
	config.ParseEnvAsStringSlice("apm_config.filter_tags_regex.reject", parseKVList("apm_config.filter_tags_regex.reject"))

	config.ParseEnvAsSliceMapString("apm_config.replace_tags", func(in string) []map[string]string {
		var out []map[string]string
		if err := json.Unmarshal([]byte(in), &out); err != nil {
			log.Warnf(`"apm_config.replace_tags" can not be parsed: %v`, err)
		}
		return out
	})

	config.ParseEnvAsMapStringInterface("apm_config.analyzed_spans", func(in string) map[string]interface{} {
		out, err := parseAnalyzedSpans(in)
		if err != nil {
			log.Errorf(`Bad format for "apm_config.analyzed_spans" it should be of the form \"service_name|operation_name=rate,other_service|other_operation=rate\", error: %v`, err)
		}
		return out
	})

	config.BindEnv("apm_config.peer_tags", "DD_APM_PEER_TAGS")
	config.ParseEnvAsStringSlice("apm_config.peer_tags", func(in string) []string {
		var out []string
		if err := json.Unmarshal([]byte(in), &out); err != nil {
			log.Warnf(`"apm_config.peer_tags" can not be parsed: %v`, err)
		}
		return out
	})
}

func parseKVList(key string) func(string) []string {
	return func(in string) []string {
		if len(in) == 0 {
			return []string{}
		}
		if in[0] != '[' {
			return strings.Split(in, " ")
		}
		// '[' as a first character signals JSON array format
		var values []string
		if err := json.Unmarshal([]byte(in), &values); err != nil {
			log.Warnf(`"%s" can not be parsed: %v`, key, err)
			return []string{}
		}
		return values
	}
}

func splitCSVString(s string, sep rune) ([]string, error) {
	r := csv.NewReader(strings.NewReader(s))
	r.TrimLeadingSpace = true
	r.LazyQuotes = true
	r.Comma = sep

	return r.Read()
}

func parseNameAndRate(token string) (string, float64, error) {
	parts := strings.Split(token, "=")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("Bad format")
	}
	rate, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return "", 0, fmt.Errorf("Unabled to parse rate")
	}
	return parts[0], rate, nil
}

// parseAnalyzedSpans parses the env string to extract a map of spans to be analyzed by service and operation.
// the format is: service_name|operation_name=rate,other_service|other_operation=rate
func parseAnalyzedSpans(env string) (map[string]interface{}, error) {
	analyzedSpans := make(map[string]interface{})
	if env == "" {
		return analyzedSpans, nil
	}
	tokens := strings.Split(env, ",")
	for _, token := range tokens {
		name, rate, err := parseNameAndRate(token)
		if err != nil {
			return nil, err
		}
		analyzedSpans[name] = rate
	}
	return analyzedSpans, nil
}
