package agentprovider

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/converters"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type confMap = map[string]any

func buildReceivers(conf confMap, agent configManager) []any {
	receivers := make(confMap)

	hostProfiler := make(confMap)
	converters.Set(hostProfiler, "symbol_uploader::enabled", true)

	symbolEndpoints := make([]any, 0, agent.endpointsTotalLength)
	for _, endpoint := range agent.endpoints {
		for _, key := range endpoint.apiKeys {
			symbolEndpoints = append(symbolEndpoints, confMap{
				"site":    endpoint.site,
				"api_key": key,
			})
		}
	}

	_ = converters.Set(hostProfiler, "symbol_uploader::symbol_endpoints", symbolEndpoints)

	receivers["hostprofiler"] = hostProfiler
	conf["receivers"] = receivers
	return []any{"hostprofiler"}
}

func buildExporters(conf confMap, agent configManager) []any {
	const (
		profilesEndpointFormat = "https://intake.profile.%s/v1development/profiles"
		metricsEndpointFormat  = "https://otlp.%s/v1/metrics"
		otlpHTTPNameFormat     = "otlphttp/%s_%d"
	)

	exporters := make(confMap)

	createOtlpHTTPFromEndpoint := func(site, key string) confMap {
		return confMap{
			"profiles_endpoint": fmt.Sprintf(profilesEndpointFormat, site),
			"metrics_endpoint":  fmt.Sprintf(metricsEndpointFormat, site),
			"headers": confMap{
				"dd-api-key": key,
			},
		}
	}

	profilesExporters := make([]any, 0, agent.endpointsTotalLength)
	for _, endpoint := range agent.endpoints {
		for i, key := range endpoint.apiKeys {
			exporterName := fmt.Sprintf(otlpHTTPNameFormat, endpoint.site, i)
			_ = converters.Set(exporters, exporterName, createOtlpHTTPFromEndpoint(endpoint.site, key))
			profilesExporters = append(profilesExporters, exporterName)
		}
	}

	conf["exporters"] = exporters
	return profilesExporters
}

func buildProcessors(conf confMap) []any {
	processors := make(confMap)

	converters.Set(processors, "infraattributes/default::allow_hostname_override", true)
	conf["processors"] = processors
	return []any{"infraattributes/default"}
}

func buildConfig(agent configManager) confMap {
	config := make(confMap)

	profilesPipeline, _ := converters.Ensure[confMap](config, "service::pipelines::profiles")

	profilesPipeline["processors"] = buildProcessors(config)
	profilesPipeline["exporters"] = buildExporters(config, agent)
	profilesPipeline["receivers"] = buildReceivers(config, agent)

	_ = converters.Set(config, "extensions::ddprofiling/default", confMap{})
	_ = converters.Set(config, "extensions::hpflare/default", confMap{})

	log.Debugf("Generated configuration: %+v", config)

	return config
}
