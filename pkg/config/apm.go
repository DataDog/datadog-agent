package config

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func setupAPM(config Config) {
	config.SetKnown("apm_config.obfuscation.elasticsearch.enabled")
	config.SetKnown("apm_config.obfuscation.elasticsearch.keep_values")
	config.SetKnown("apm_config.obfuscation.elasticsearch.obfuscate_sql_values")
	config.SetKnown("apm_config.obfuscation.mongodb.enabled")
	config.SetKnown("apm_config.obfuscation.mongodb.keep_values")
	config.SetKnown("apm_config.obfuscation.mongodb.obfuscate_sql_values")
	config.SetKnown("apm_config.obfuscation.sql_exec_plan.enabled")
	config.SetKnown("apm_config.obfuscation.sql_exec_plan.keep_values")
	config.SetKnown("apm_config.obfuscation.sql_exec_plan.obfuscate_sql_values")
	config.SetKnown("apm_config.obfuscation.sql_exec_plan_normalize.enabled")
	config.SetKnown("apm_config.obfuscation.sql_exec_plan_normalize.keep_values")
	config.SetKnown("apm_config.obfuscation.sql_exec_plan_normalize.obfuscate_sql_values")
	config.SetKnown("apm_config.obfuscation.http.remove_query_string")
	config.SetKnown("apm_config.obfuscation.http.remove_paths_with_digits")
	config.SetKnown("apm_config.obfuscation.remove_stack_traces")
	config.SetKnown("apm_config.obfuscation.redis.enabled")
	config.SetKnown("apm_config.obfuscation.memcached.enabled")
	config.SetKnown("apm_config.filter_tags.require")
	config.SetKnown("apm_config.filter_tags.reject")
	config.SetKnown("apm_config.extra_sample_rate")
	config.SetKnown("apm_config.dd_agent_bin")
	config.SetKnown("apm_config.trace_writer.connection_limit")
	config.SetKnown("apm_config.trace_writer.queue_size")
	config.SetKnown("apm_config.service_writer.connection_limit")
	config.SetKnown("apm_config.service_writer.queue_size")
	config.SetKnown("apm_config.stats_writer.connection_limit")
	config.SetKnown("apm_config.stats_writer.queue_size")
	config.SetKnown("apm_config.analyzed_rate_by_service.*")
	config.SetKnown("apm_config.log_throttling")
	config.SetKnown("apm_config.bucket_size_seconds")
	config.SetKnown("apm_config.watchdog_check_delay")
	config.SetKnown("apm_config.sync_flushing")

	if runtime.GOARCH == "386" && runtime.GOOS == "windows" {
		// on Windows-32 bit, the trace agent isn't installed.  Set the default to disabled
		// so that there aren't messages in the log about failing to start.
		config.BindEnvAndSetDefault("apm_config.enabled", false, "DD_APM_ENABLED")
	} else {
		config.BindEnvAndSetDefault("apm_config.enabled", true, "DD_APM_ENABLED")
	}

	config.BindEnvAndSetDefault("apm_config.receiver_port", 8126, "DD_APM_RECEIVER_PORT", "DD_RECEIVER_PORT")
	config.BindEnvAndSetDefault("apm_config.windows_pipe_buffer_size", 1_000_000, "DD_APM_WINDOWS_PIPE_BUFFER_SIZE")                          //nolint:errcheck
	config.BindEnvAndSetDefault("apm_config.windows_pipe_security_descriptor", "D:AI(A;;GA;;;WD)", "DD_APM_WINDOWS_PIPE_SECURITY_DESCRIPTOR") //nolint:errcheck
	config.BindEnvAndSetDefault("apm_config.remote_tagger", false, "DD_APM_REMOTE_TAGGER")                                                    //nolint:errcheck

	config.BindEnv("apm_config.max_catalog_services", "DD_APM_MAX_CATALOG_SERVICES")
	config.BindEnv("apm_config.receiver_timeout", "DD_APM_RECEIVER_TIMEOUT")
	config.BindEnv("apm_config.max_payload_size", "DD_APM_MAX_PAYLOAD_SIZE")
	config.BindEnv("apm_config.log_file", "DD_APM_LOG_FILE")
	config.BindEnv("apm_config.max_events_per_second", "DD_APM_MAX_EPS", "DD_MAX_EPS")
	config.BindEnv("apm_config.max_traces_per_second", "DD_APM_MAX_TPS", "DD_MAX_TPS")
	config.BindEnv("apm_config.max_memory", "DD_APM_MAX_MEMORY")
	config.BindEnv("apm_config.max_cpu_percent", "DD_APM_MAX_CPU_PERCENT")
	config.BindEnv("apm_config.env", "DD_APM_ENV")
	config.BindEnv("apm_config.apm_non_local_traffic", "DD_APM_NON_LOCAL_TRAFFIC")
	config.BindEnv("apm_config.apm_dd_url", "DD_APM_DD_URL")
	config.BindEnv("apm_config.connection_limit", "DD_APM_CONNECTION_LIMIT", "DD_CONNECTION_LIMIT")
	config.BindEnv("apm_config.connection_reset_interval", "DD_APM_CONNECTION_RESET_INTERVAL")
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
	config.BindEnv("apm_config.internal_profiling.enabled", "DD_APM_INTERNAL_PROFILING_ENABLED")
	config.BindEnv("apm_config.debugger_dd_url", "DD_APM_DEBUGGER_DD_URL")
	config.BindEnv("experimental.otlp.http_port", "DD_OTLP_HTTP_PORT")
	config.BindEnv("experimental.otlp.grpc_port", "DD_OTLP_GRPC_PORT")

	config.SetEnvKeyTransformer("apm_config.ignore_resources", func(in string) interface{} {
		r, err := splitCSVString(in, ',')
		if err != nil {
			log.Warnf(`"apm_config.ignore_resources" can not be parsed: %v`, err)
			return []string{}
		}
		return r
	})

	config.SetEnvKeyTransformer("apm_config.filter_tags.require", func(in string) interface{} {
		return strings.Split(in, " ")
	})

	config.SetEnvKeyTransformer("apm_config.filter_tags.reject", func(in string) interface{} {
		return strings.Split(in, " ")
	})

	config.SetEnvKeyTransformer("apm_config.replace_tags", func(in string) interface{} {
		var out []map[string]string
		if err := json.Unmarshal([]byte(in), &out); err != nil {
			log.Warnf(`"apm_config.replace_tags" can not be parsed: %v`, err)
		}
		return out
	})

	config.SetEnvKeyTransformer("apm_config.analyzed_spans", func(in string) interface{} {
		out, err := parseAnalyzedSpans(in)
		if err != nil {
			log.Errorf(`Bad format for "apm_config.analyzed_spans" it should be of the form \"service_name|operation_name=rate,other_service|other_operation=rate\", error: %v`, err)
		}
		return out
	})
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
func parseAnalyzedSpans(env string) (analyzedSpans map[string]interface{}, err error) {
	analyzedSpans = make(map[string]interface{})
	if env == "" {
		return
	}
	tokens := strings.Split(env, ",")
	for _, token := range tokens {
		name, rate, err := parseNameAndRate(token)
		if err != nil {
			return nil, err
		}
		analyzedSpans[name] = rate
	}
	return
}
