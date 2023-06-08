package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/otel-collector/pkg/extensions/trace"
	otelcomponents "github.com/DataDog/datadog-agent/otel-collector/pkg/otel-components"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	zapAgent "github.com/DataDog/datadog-agent/pkg/util/log/zap"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/otelcol"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	buildInfo := component.BuildInfo{
		Command:     "dd-otel-collector",
		Description: "DD OTel Collector",
		Version:     "0.1",
	}
	flagSet, err := buildAndParseFlagSet(featuregate.GlobalRegistry())
	if err != nil {
		log.Fatal(fmt.Errorf("failed to build components: %w", err))
	}
	if err = runInteractive(buildInfo, flagSet); err != nil {
		log.Fatal(fmt.Errorf("failed to run: %w", err))
	}
}

func regToFlagSet(reg *featuregate.Registry) *flag.FlagSet {
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

// We parse the flags manually here so that we can use feature gates when constructing
// our default component list. Flags also need to be parsed before creating the config provider.
func buildAndParseFlagSet(featgate *featuregate.Registry) (*flag.FlagSet, error) {
	flagSet := regToFlagSet(featgate)

	if err := flagSet.Parse(os.Args[1:]); err != nil {
		return nil, err
	}
	return flagSet, nil
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

func runInteractive(buildInfo component.BuildInfo, flagSet *flag.FlagSet) error {
	cmd := newCommand(buildInfo, flagSet)
	err := cmd.Execute()
	if err != nil {
		return fmt.Errorf("application run finished with error: %w", err)
	}
	return nil
}

// newCommand constructs a new cobra.Command using the given settings.
func newCommand(buildInfo component.BuildInfo, flagSet *flag.FlagSet) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          buildInfo.Command,
		Version:      buildInfo.Version,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			loc := getConfigFlag(flagSet)
			provider, err := otelcomponents.ConfigProvider(loc)
			if err != nil {
				return err
			}
			conf := otelcomponents.NewOtelConfig[trace.APIConfig](trace.APIConfig{})
			return fxutil.OneShot(start,
				fx.Supply(fx.Annotate(provider, fx.As(new(otelcol.ConfigProvider)))),
				fx.Supply(buildInfo),
				fx.Supply(logComponent.LogForOneShot("", "debug", false)),
				fx.Supply(fx.Annotate(conf, fx.As(new(config.Component)))),
				logComponent.Module,
				fx.Provide(func() []zap.Option {
					return []zap.Option{
						zap.WrapCore(func(zapcore.Core) zapcore.Core {
							return zapAgent.NewZapCore()
						}),
					}
				}),
				fx.Provide(AsExtension(trace.NewFactory)),
				fx.Provide(fx.Annotate(otelcomponents.GetExtensionMap, fx.ParamTags(`group:"extension"`))),
				fx.Provide(otelcomponents.GetProcessors),
				fx.Provide(otelcomponents.GetExporters),
				fx.Provide(otelcomponents.GetReceivers),
				fx.Provide(otelcomponents.NewFactory),
				fx.Provide(otelcomponents.NewCollector),
			)
		},
	}
	rootCmd.Flags().AddGoFlagSet(flagSet)
	return rootCmd
}

func AsExtension(f any) any {
	return fx.Annotate(f, fx.As(new(extension.Factory)), fx.ResultTags(`group:"extension"`))
}

func start(col *otelcol.Collector) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return col.Run(ctx)
}
