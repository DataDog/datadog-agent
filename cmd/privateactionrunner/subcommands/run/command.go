// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package run is the run private-action-runner subcommand
package run

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauthnooptypes "github.com/DataDog/datadog-agent/comp/core/delegatedauth/noop-impl/types"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsimpl "github.com/DataDog/datadog-agent/comp/core/secrets/impl"
	secretnooptypes "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl/types"
	settings "github.com/DataDog/datadog-agent/comp/core/settings/def"
	settingsfx "github.com/DataDog/datadog-agent/comp/core/settings/fx"
	telemetrynoopsimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformfx "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/fx"
	eventplatformreceiverimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/impl"
	remotetraceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/fx-remote"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	privateactionrunnerfx "github.com/DataDog/datadog-agent/comp/privateactionrunner/fx"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	rcclientfx "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/fx"
	rcservicefx "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/fx"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	pkgconfigcreate "github.com/DataDog/datadog-agent/pkg/config/create"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type cliParams struct {
	*command.GlobalParams
}

// runPrivateActionRunner runs the private action runner with the given configuration and context.
// This function is shared between the CLI run command and the Windows service.
func runPrivateActionRunner(ctx context.Context, confPath string, extraConfFiles []string) error {
	enabled, err := parEnabled(confPath, extraConfFiles)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}

	fxOptions := []fx.Option{
		// Provide context for cancellation (Windows service uses this for graceful shutdown)
		fx.Provide(func() context.Context { return ctx }),
		// Setup shutdown listener for context cancellation (e.g., from Windows SCM)
		fx.Invoke(func(shutdowner fx.Shutdowner) {
			go func() {
				<-ctx.Done()
				_ = shutdowner.Shutdown()
			}()
		}),
		fx.Supply(core.BundleParams{
			ConfigParams: config.NewAgentParams(confPath, config.WithExtraConfFiles(extraConfFiles)),
			LogParams:    log.ForDaemon(command.LoggerName, pkgconfigsetup.PARLogFile, pkgconfigsetup.DefaultPrivateActionRunnerLogFile)}),
		core.Bundle(core.WithSecrets()),
		fx.Provide(func(c config.Component) settings.Params {
			return settings.Params{
				Settings: map[string]settings.RuntimeSetting{
					"log_level": commonsettings.NewLogLevelRuntimeSetting(),
				},
				Config: c,
			}
		}),
		settingsfx.Module(),
		remotehostnameimpl.Module(),
		ipcfx.ModuleReadWrite(),
		rcservicefx.Module(),
		rcclientfx.Module(),
		fx.Supply(rcclient.Params{AgentName: "private-action-runner", AgentVersion: version.AgentVersion}),
		getTaggerModule(),
		remotetraceroute.Module(),
		logscompressionfx.Module(),
		eventplatformreceiverimpl.Module(),
		eventplatformfx.Module(eventplatform.NewDefaultParams()),
		privateactionrunnerfx.Module(),
	}

	err = fxutil.Run(fxOptions...)
	if errors.Is(err, privateactionrunner.ErrNotEnabled) {
		return nil
	}
	return err
}

// parEnabled loads enough config to decide whether the PAR Fx graph should be built.
func parEnabled(confPath string, extraConfFiles []string) (bool, error) {
	return parEnabledWithSecretResolver(confPath, extraConfFiles, func() secrets.Component {
		return secretsimpl.NewEnabledResolver(telemetrynoopsimpl.GetCompatComponent())
	})
}

func parEnabledWithSecretResolver(confPath string, extraConfFiles []string, newSecretResolver func() secrets.Component) (bool, error) {
	cfg, err := loadPARPreflightConfig(confPath, extraConfFiles, &secretnooptypes.SecretNoop{})
	if err != nil {
		return false, err
	}
	if parEnablementUsesSecret(cfg) {
		cfg, err = loadPARPreflightConfig(confPath, extraConfFiles, newSecretResolver())
		if err != nil {
			return false, err
		}
	}
	return privateactionrunner.IsEnabled(cfg), nil
}

func loadPARPreflightConfig(confPath string, extraConfFiles []string, secretResolver secrets.Component) (pkgconfigmodel.BuildableConfig, error) {
	cfg := pkgconfigcreate.NewConfig("datadog", "")
	pkgconfigsetup.InitConfig(cfg)
	cfg.BuildSchema()

	if confPath != "" {
		cfg.AddConfigPath(confPath)
		if strings.HasSuffix(confPath, ".yaml") || strings.HasSuffix(confPath, ".yml") {
			cfg.SetConfigFile(confPath)
		}
	}
	if defaultConfPath := defaultpaths.GetDefaultConfPath(); defaultConfPath != "" {
		cfg.AddConfigPath(defaultConfPath)
	}
	if err := cfg.AddExtraConfigPaths(extraConfFiles); err != nil {
		return nil, err
	}

	err := pkgconfigsetup.LoadDatadog(cfg, secretResolver, &delegatedauthnooptypes.DelegatedAuthNoop{}, pkgconfigsetup.SystemProbe().GetEnvVars())
	if err != nil && (!errors.Is(err, pkgconfigmodel.ErrConfigFileNotFound) || confPath != "") {
		return nil, fmt.Errorf("unable to load Datadog config file: %w", err)
	}

	if fleetPoliciesDirPath := cfg.GetString("fleet_policies_dir"); fleetPoliciesDirPath != "" {
		if err := cfg.MergeFleetPolicy(path.Join(fleetPoliciesDirPath, "datadog.yaml")); err != nil {
			return nil, err
		}
		pkgconfigsetup.ApplyUseDogstatsdSuppression(cfg)
	}

	return cfg, nil
}

func parEnablementUsesSecret(cfg pkgconfigmodel.Reader) bool {
	value, ok := cfg.Get(privateactionrunner.PAREnabled).(string)
	if !ok {
		return false
	}
	value = strings.Trim(value, " \t")
	return strings.HasPrefix(value, "ENC[") && strings.HasSuffix(value, "]")
}

// Commands returns a slice of subcommands for the 'private-action-runner' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run the Private Action Runner",
		Long:  `Runs the private-action-runner in the foreground`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runPrivateActionRunner(context.Background(), globalParams.ConfFilePath, cliParams.ExtraConfFilePath)
		},
	}

	return []*cobra.Command{runCmd}
}
