package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
)

// NewConfigComponent creates a new config component from the given URIs
func NewConfigComponent(uris []string) (config.Component, error) {
	// Load the configuration from the fileName
	set := confmap.ProviderSettings{}
	rs := confmap.ResolverSettings{
		URIs: uris,
		Providers: makeMapProvidersMap(
			fileprovider.NewWithSettings(set),
			envprovider.NewWithSettings(set),
			yamlprovider.NewWithSettings(set),
			httpprovider.NewWithSettings(set),
		),
		Converters: []confmap.Converter{expandconverter.New(confmap.ConverterSettings{})},
	}
	fmt.Printf("Loading config from %s\n", uris)

	resolver, err := confmap.NewResolver(rs)
	if err != nil {
		fmt.Printf("Error creating resolver: %v\n", err)
		return nil, err
	}
	cfg, err := resolver.Resolve(context.Background())
	if err != nil {
		fmt.Printf("Error resolving config: %v\n", err)
		return nil, err
	}
	var configs []*datadogexporter.Config
	for k, v := range cfg.ToStringMap() {
		if k != "exporters" {
			continue
		}
		exporters := v.(map[string]any)
		for k, v := range exporters {
			if strings.HasPrefix(k, "datadog") {
				var datadogConfig *datadogexporter.Config
				m := v.(map[string]any)
				err = confmap.NewFromStringMap(m).Unmarshal(&datadogConfig)
				if err != nil {
					return nil, err
				}
				configs = append(configs, datadogConfig)
			}
		}
	}
	if len(configs) == 0 {
		return nil, fmt.Errorf("no datadog exporter found in %s", uris)
	}
	// Ensure datadog exporter has same apikey
	apiKey := string(configs[0].API.Key)
	site := configs[0].API.Site

	for _, c := range configs {
		if string(c.API.Key) != apiKey || c.API.Site != site {
			return nil, fmt.Errorf("datadog exporter has different apikey or site")
		}
	}
	// Create a new config
	pkgconfig := pkgconfigmodel.NewConfig("OTel", "DD", strings.NewReplacer(".", "_"))
	// Set Default values
	pkgconfigsetup.InitConfig(pkgconfig)
	pkgconfig.Set("api_key", apiKey, pkgconfigmodel.SourceFile)
	pkgconfig.Set("site", site, pkgconfigmodel.SourceFile)
	pkgconfig.Set("logs_enabled", true, pkgconfigmodel.SourceFile)
	pkgconfig.Set("logs_config.use_compression", true, pkgconfigmodel.SourceFile)
	// TODO: set the correct value
	pkgconfig.Set("log_level", "info", pkgconfigmodel.SourceFile)
	pkgconfig.Set("forwarder_timeout", 10, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("apm_config.enabled", true, pkgconfigmodel.SourceFile)
	pkgconfig.Set("apm_config.apm_non_local_traffic", true, pkgconfigmodel.SourceFile)
	return pkgconfig, nil
}

func makeMapProvidersMap(providers ...confmap.Provider) map[string]confmap.Provider {
	ret := make(map[string]confmap.Provider, len(providers))
	for _, provider := range providers {
		ret[provider.Scheme()] = provider
	}
	return ret
}
