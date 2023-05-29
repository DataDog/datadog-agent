package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpsprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/otelcol"

	"go.opentelemetry.io/collector/component"

	"github.com/DataDog/datadog-agent/otel-collector/pkg/defaultcomponents"
)

func main() {
	// get extra config

	info := component.BuildInfo{
		Command:     "dd-otel-collector",
		Description: "DD OTel Collector",
		Version:     "0.1",
	}

	flagSet, err := buildAndParseFlagSet(featuregate.GlobalRegistry())
	if err != nil {
		log.Fatal(err)
	}

	factories, err := defaultcomponents.Components()

	if err != nil {
		log.Fatal(fmt.Errorf("failed to build components: %w", err))
	}

	providers := []confmap.Provider{
		fileprovider.New(),
		envprovider.New(),
		yamlprovider.New(),
		httpprovider.New(),
		httpsprovider.New(),
	}

	mapProviders := make(map[string]confmap.Provider, len(providers))
	for _, provider := range providers {
		mapProviders[provider.Scheme()] = provider
	}

	loc := getConfigFlag(flagSet)
	// create Config Provider Settings
	settings := otelcol.ConfigProviderSettings{
		ResolverSettings: confmap.ResolverSettings{
			URIs:       loc,
			Providers:  mapProviders,
			Converters: []confmap.Converter{expandconverter.New()},
		},
	}

	// get New config Provider
	configProvider, err := otelcol.NewConfigProvider(settings)
	params := otelcol.CollectorSettings{
		Factories:      factories,
		BuildInfo:      info,
		ConfigProvider: configProvider,
	}

	if err = runInteractive(params, flagSet); err != nil {
		log.Fatal(err)
	}
}

type configFlagValue struct {
	values []string
	sets   []string
}

func (s *configFlagValue) Set(val string) error {
	s.values = append(s.values, val)
	return nil
}

func (s *configFlagValue) String() string {
	return "[" + strings.Join(s.values, ", ") + "]"
}

const (
	configFlag       = "config"
	featureGatesFlag = "feature-gates"
)

func getConfigFlag(flagSet *flag.FlagSet) []string {
	cfv := flagSet.Lookup(configFlag).Value.(*configFlagValue)
	return append(cfv.values, cfv.sets...)
}

// We parse the flags manually here so that we can use feature gates when constructing
// our default component list. Flags also need to be parsed before creating the config provider.
func buildAndParseFlagSet(featgate *featuregate.Registry) (*flag.FlagSet, error) {
	flagSet := Flags(featgate)

	if err := flagSet.Parse(os.Args[1:]); err != nil {
		return nil, err
	}
	return flagSet, nil
}

func Flags(reg *featuregate.Registry) *flag.FlagSet {
	flagSet := new(flag.FlagSet)

	cfgs := new(configFlagValue)
	flagSet.Var(cfgs, configFlag, "Locations to the config file(s), note that only a"+
		" single location can be set per flag entry e.g. `--config=file:/path/to/first --config=file:path/to/second`.")

	flagSet.Func("set",
		"Set arbitrary component config property. The component has to be defined in the config file and the flag"+
			" has a higher precedence. Array config properties are overridden and maps are joined. Example --set=processors.batch.timeout=2s",
		func(s string) error {
			idx := strings.Index(s, "=")
			if idx == -1 {
				// No need for more context, see TestSetFlag/invalid_set.
				return errors.New("missing equal sign")
			}
			cfgs.sets = append(cfgs.sets, "yaml:"+strings.TrimSpace(strings.ReplaceAll(s[:idx], ".", "::"))+": "+strings.TrimSpace(s[idx+1:]))
			return nil
		})

	flagSet.Var(featuregate.NewFlag(reg), featureGatesFlag,
		"Comma-delimited list of feature gate identifiers. Prefix with '-' to disable the feature. '+' or no prefix will enable the feature.")

	return flagSet
}
func runInteractive(params otelcol.CollectorSettings, flagSet *flag.FlagSet) error {
	cmd := newCommand(params, flagSet)
	err := cmd.Execute()
	if err != nil {
		return fmt.Errorf("application run finished with error: %w", err)
	}
	return nil
}

// newCommand constructs a new cobra.Command using the given settings.
func newCommand(params otelcol.CollectorSettings, flagSet *flag.FlagSet) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          params.BuildInfo.Command,
		Version:      params.BuildInfo.Version,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			col, err := otelcol.NewCollector(params)
			if err != nil {
				return fmt.Errorf("failed to construct the application: %w", err)
			}
			return col.Run(cmd.Context())
		},
	}

	rootCmd.Flags().AddGoFlagSet(flagSet)
	return rootCmd
}
