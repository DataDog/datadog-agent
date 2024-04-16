// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bootstrap implements 'installer bootstrap'.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-ini/ini"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/installer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

type cliParams struct {
	command.GlobalParams
	url     string
	pkg     string
	version string
}

var (
	installScriptParamsFile = filepath.Join(path.DefaultConfPath, "datadog-installer.ini")
)

type installScriptParams struct {
	Telemetry installScriptTelemetryParams `ini:"telemetry"`
	Features  installScriptFeaturesParams  `ini:"features"`
}

type installScriptTelemetryParams struct {
	TraceID  uint64 `ini:"trace_id"`
	ParentID uint64 `ini:"parent_id"`
	Priority int    `ini:"priority"`
}

type installScriptFeaturesParams struct {
	APMInstrumentation string `ini:"apm_instrumentation"`
}

// Commands returns the bootstrap command
func Commands(global *command.GlobalParams) []*cobra.Command {
	var timeout time.Duration
	var url string
	var pkg string
	var version string
	bootstrapCmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstraps the package with the first version.",
		Long: `Installs the first version of the package managed by the installer.
		This first version is sent remotely to the agent and can be configured from the UI.
		This command will exit after the first version is installed.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			installScriptParams, err := readInstallScriptParams()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			return bootstrapFxWrapper(ctx, &cliParams{
				GlobalParams: *global,
				url:          url,
				pkg:          pkg,
				version:      version,
			}, installScriptParams)
		},
	}
	bootstrapCmd.Flags().DurationVarP(&timeout, "timeout", "T", 3*time.Minute, "timeout to bootstrap with")
	bootstrapCmd.Flags().StringVarP(&url, "url", "u", "", "URL to fetch the package from")
	bootstrapCmd.Flags().StringVarP(&pkg, "package", "P", "", "Package name to bootstrap")
	bootstrapCmd.Flags().StringVarP(&version, "version", "V", "latest", "Version to bootstrap")
	return []*cobra.Command{bootstrapCmd}
}

func bootstrapFxWrapper(ctx context.Context, params *cliParams, installScriptParams *installScriptParams) error {
	return fxutil.OneShot(bootstrap,
		fx.Provide(func() context.Context { return ctx }),
		fx.Supply(params),
		fx.Supply(installScriptParams),
		fx.Supply(core.BundleParams{
			ConfigParams:         config.NewAgentParams(params.GlobalParams.ConfFilePath),
			SecretParams:         secrets.NewEnabledParams(),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(),
			LogParams:            logimpl.ForOneShot("INSTALLER", "info", true),
		}),
		core.Bundle(),
		telemetryimpl.Module(),
	)
}

func bootstrap(ctx context.Context, params *cliParams, installScriptParams *installScriptParams, config config.Component, log log.Component, _ telemetry.Component) error {
	ctxCarrier := tracer.TextMapCarrier{
		tracer.DefaultTraceIDHeader:  fmt.Sprint(installScriptParams.Telemetry.TraceID),
		tracer.DefaultParentIDHeader: fmt.Sprint(installScriptParams.Telemetry.ParentID),
		tracer.DefaultPriorityHeader: fmt.Sprint(installScriptParams.Telemetry.Priority),
	}
	spanCtx, err := tracer.Extract(ctxCarrier)
	if err != nil {
		log.Errorf("failed to extract span context from install script params: %v", err)
	}
	span, ctx := tracer.StartSpanFromContext(ctx, "cmd/bootstrap", tracer.ChildOf(spanCtx))
	defer span.Finish()
	span.SetTag(ext.ManualKeep, true)
	span.SetTag("params.pkg", params.pkg)
	span.SetTag("params.version", params.version)
	span.SetTag("script_params.telemetry.trace_id", installScriptParams.Telemetry.TraceID)
	span.SetTag("script_params.telemetry.span_id", installScriptParams.Telemetry.ParentID)
	span.SetTag("script_params.features.apm_instrumentation", installScriptParams.Features.APMInstrumentation)

	if params.pkg == "" && params.url == "" {
		return installer.Bootstrap(ctx, config)
	}

	url := packageURL(config.GetString("site"), params.pkg, params.version)
	if params.url != "" {
		url = params.url
	}
	span.SetTag("params.url", params.url)
	return installer.BootstrapURL(ctx, url, config)
}

func packageURL(site string, pkg string, version string) string {
	if site == "datad0g.com" {
		return stagingPackageURL(pkg, version)
	}
	return prodPackageURL(pkg, version)
}

func stagingPackageURL(pkg string, version string) string {
	return fmt.Sprintf("oci://docker.io/datadog/%s-package-dev:%s", strings.TrimPrefix(pkg, "datadog-"), version)
}

func prodPackageURL(pkg string, version string) string {
	return fmt.Sprintf("oci://public.ecr.aws/datadoghq/%s-package:%s", strings.TrimPrefix(pkg, "datadog-"), version)
}

func readInstallScriptParams() (*installScriptParams, error) {
	params := &installScriptParams{}
	file, err := os.Open(installScriptParamsFile)
	if errors.Is(err, os.ErrNotExist) {
		return params, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open install script params file: %w", err)
	}
	defer file.Close()
	cfg, err := ini.Load(file)
	if err != nil {
		return nil, fmt.Errorf("failed to load install script params file: %w", err)
	}
	err = cfg.MapTo(params)
	if err != nil {
		return nil, fmt.Errorf("failed to map install script params: %w", err)
	}
	return params, nil
}
