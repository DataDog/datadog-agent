// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package converterimpl

import (
	"fmt"
	"net/url"
	"strings"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/service/telemetry"
)

var (
	// otlp receiver for internal metrics
	otlpName         = "otlp"
	otlpEnhancedName = otlpName + "/" + ddAutoconfiguredSuffix
	otlpAddrDefault  = "localhost:15678"
	otlpConfig       = map[string]any{
		"protocols": map[string]any{
			"http": map[string]any{
				"endpoint": otlpAddrDefault,
			},
		},
	}
	otlpRecvDefault = component{
		Name:         otlpName,
		EnhancedName: otlpEnhancedName,
		Type:         "receivers",
		Config:       otlpConfig,
	}
)

// configureInternalMetrics sets up the internal metrics components and pipelines in DDOT
// see https://docs.google.com/document/d/1KlsN-DhPYIHoRr69mhsB-aEDATpZxC2pg7nBKQbP7aI/edit?pli=1&tab=t.0 for details in implementation
func configureInternalMetrics(conf *confmap.Conf) {
	mLevel := conf.Get("service::telemetry::metrics::level")
	if mLevel != nil && strings.ToLower(mLevel.(string)) == "none" {
		return
	}

	if conf.Get("service::telemetry::metrics::readers") != nil {
		configureExistingMetricReaders(conf)
		return
	}

	if hasPromReceiverForDefaultInternalMetrics(conf) {
		addPrometheusReceiver(conf, defaultPromServerAddr)
		return
	}

	addDefaultPeriodicOTLPReader(conf)
	addComponentToConfig(conf, otlpRecvDefault)
	addDDExpToInternalPipeline(conf, otlpRecvDefault, getDatadogExporters(conf))
}

func configureExistingMetricReaders(conf *confmap.Conf) {
	svcTelemCfgRaw := conf.Get("service::telemetry")
	svcTelemCfgMap, ok := svcTelemCfgRaw.(map[string]any)
	if !ok {
		return
	}
	svcTelemConf := confmap.NewFromStringMap(svcTelemCfgMap)
	var svcTelemCfg telemetry.Config
	if err := svcTelemCfg.Unmarshal(svcTelemConf); err != nil {
		fmt.Println("invalid service telemetry configs", err)
		return
	}
	if err := svcTelemCfg.Validate(); err != nil {
		fmt.Println("invalid service telemetry configs", err)
		return
	}

	for _, mreader := range svcTelemCfg.Metrics.Readers {
		if mreader.Periodic != nil && mreader.Periodic.Exporter.OTLP != nil {
			otlpExpCfg := mreader.Periodic.Exporter.OTLP
			handleExistingPeriodicOTLPExporter(conf, *otlpExpCfg.Endpoint, *otlpExpCfg.Protocol)
			return
		}
		if mreader.Pull != nil && mreader.Pull.Exporter.Prometheus != nil {
			promCfg := mreader.Pull.Exporter.Prometheus
			addPrometheusReceiver(conf, fmt.Sprintf("%s:%d", *promCfg.Host, *promCfg.Port))
			return
		}
	}
}

func addDefaultPeriodicOTLPReader(conf *confmap.Conf) {
	stringMapConf := conf.ToStringMap()
	service, ok := stringMapConf["service"]
	if !ok {
		return
	}
	serviceMap, ok := service.(map[string]any)
	if !ok {
		return
	}
	_, ok = serviceMap["telemetry"]
	if !ok {
		serviceMap["telemetry"] = make(map[string]any)
	}
	svcTelemMap, ok := serviceMap["telemetry"].(map[string]any)
	if !ok {
		return
	}
	_, ok = svcTelemMap["metrics"]
	if !ok {
		svcTelemMap["metrics"] = make(map[string]any)
	}
	svcTelemMtrcMap, ok := svcTelemMap["metrics"].(map[string]any)
	if !ok {
		return
	}
	_, ok = svcTelemMtrcMap["readers"]
	if !ok {
		svcTelemMtrcMap["readers"] = make([]any, 0, 1)
	}
	mreaders, ok := svcTelemMtrcMap["readers"].([]any)
	if !ok {
		return
	}
	mreaders = append(mreaders, map[string]any{
		"periodic": map[string]any{
			"exporter": map[string]any{
				"otlp": map[string]any{
					"protocol": "http/protobuf",
					"endpoint": otlpAddrDefault,
				},
			},
		},
	})
	svcTelemMtrcMap["readers"] = mreaders
	*conf = *confmap.NewFromStringMap(stringMapConf)
}

func handleExistingPeriodicOTLPExporter(conf *confmap.Conf, endpoint string, protocol string) {
	otlpRecvComp := findMatchingOTLPReceiver(conf, endpoint, protocol)
	if otlpRecvComp != nil {
		ddExps := getDatadogExporters(conf)
		ddExpsCfged := receiverInPipelineWithDatadogExporter(conf, otlpRecvComp.EnhancedName)
		for _, ddExp := range ddExpsCfged {
			delete(ddExps, ddExp)
		}
		addDDExpToInternalPipeline(conf, *otlpRecvComp, ddExps)
		return
	}

	addDefaultPeriodicOTLPReader(conf)
	addComponentToConfig(conf, otlpRecvDefault)
	addDDExpToInternalPipeline(conf, otlpRecvDefault, getDatadogExporters(conf))
}

// findMatchingOTLPReceiver finds and returns the OTLP receiver config if a receiver matches the given endpoint and protocol
func findMatchingOTLPReceiver(conf *confmap.Conf, endpoint string, protocol string) *component {
	if strings.HasPrefix(protocol, "http") {
		protocol = "http" // http/protobuf and http/json are both served by the http receiver
	}
	cfgsByRecv := findComps(conf.ToStringMap(), otlpName, "receivers")
	for recvName, recvCfg := range cfgsByRecv {
		protocols, ok := recvCfg["protocols"]
		if !ok {
			continue
		}
		protocolsMap, ok := protocols.(map[string]any)
		if !ok {
			continue
		}
		prcl, ok := protocolsMap[protocol]
		if !ok {
			continue
		}
		prclMap, ok := prcl.(map[string]any)
		if !ok {
			continue
		}
		edpt, ok := prclMap["endpoint"]
		if !ok {
			continue
		}
		endpoint2, ok := edpt.(string)
		if !ok {
			continue
		}
		if endpointsEqual(endpoint, endpoint2) {
			return &component{
				Name:         otlpName,
				EnhancedName: recvName,
				Type:         "receivers",
				Config:       recvCfg,
			}
		}
	}

	return nil
}

// endpointsEqual compares whether two endpoints from OTLP configs are equivalent
func endpointsEqual(endpoint1, endpoint2 string) bool {
	url1 := endpointToURL(endpoint1)
	url2 := endpointToURL(endpoint2)
	return url1 != nil && url2 != nil && url1.Scheme == url2.Scheme && url1.Host == url2.Host && url1.Path == url2.Path
}

// endpointToURL converts an endpoint string to an URL. It prepends http as the default scheme if no scheme is present.
func endpointToURL(endpoint string) *url.URL {
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	url, err := url.Parse(endpoint)
	if err != nil {
		return nil
	}
	return url
}

// hasPromReceiverForDefaultInternalMetrics returns whether the collector config
// contains a Prometheus receiver that scrapes the default Prometheus address of internal metrics
func hasPromReceiverForDefaultInternalMetrics(conf *confmap.Conf) bool {
	promRecvs := findComps(conf.ToStringMap(), "prometheus", "receivers")
	for _, recvCfg := range promRecvs {
		prometheusConfig, ok := recvCfg["config"]
		if !ok {
			continue
		}
		prometheusConfigMap, ok := prometheusConfig.(map[string]any)
		if !ok {
			continue
		}
		prometheusScrapeConfigs, ok := prometheusConfigMap["scrape_configs"]
		if !ok {
			continue
		}
		prometheusScrapeConfigsSlice, ok := prometheusScrapeConfigs.([]any)
		if !ok {
			continue
		}
		for _, scrapeConfig := range prometheusScrapeConfigsSlice {
			scrapeConfigMap, ok := scrapeConfig.(map[string]any)
			if !ok {
				continue
			}
			staticConfig, ok := scrapeConfigMap["static_configs"]
			if !ok {
				continue
			}
			staticConfigSlice, ok := staticConfig.([]any)
			if !ok {
				continue
			}
			for _, staticConfig := range staticConfigSlice {
				staticConfigMap, ok := staticConfig.(map[string]any)
				if !ok {
					continue
				}
				targets, ok := staticConfigMap["targets"]
				if !ok {
					continue
				}
				targetsSlice, ok := targets.([]any)
				if !ok {
					continue
				}
				for _, target := range targetsSlice {
					targetString, ok := target.(string)
					if !ok {
						continue
					}
					if targetString == defaultPromServerAddr || targetString == "localhost:8888" {
						return true
					}
				}
			}
		}
	}
	return false
}
