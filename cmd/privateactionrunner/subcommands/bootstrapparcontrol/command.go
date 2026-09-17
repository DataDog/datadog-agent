// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package bootstrapparcontrol implements the bootstrap-par-control subcommand.
package bootstrapparcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	par "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	"github.com/DataDog/datadog-agent/pkg/fips"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	parutil "github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// Identity is the runner identity par-control signs OPMS requests with.
type Identity struct {
	URN        string `json:"urn"`
	PrivateKey string `json:"private_key"`
	OrgID      int64  `json:"org_id"`
	RunnerID   string `json:"runner_id"`
}

// ControlPlaneConfig is the resolved configuration consumed by par-control.
type ControlPlaneConfig struct {
	SplitMode bool   `json:"split_mode"`
	LogLevel  string `json:"log_level"`

	Identity *Identity      `json:"identity,omitempty"`
	Runtime  *RuntimeConfig `json:"runtime,omitempty"`

	IPCCertFilePath string `json:"ipc_cert_file_path,omitempty"`
}

// RuntimeConfig contains only the PAR-local settings consumed by the control plane.
type RuntimeConfig struct {
	OPMSBaseURL        string            `json:"opms_base_url"`
	TaskConcurrency    int32             `json:"task_concurrency"`
	ExecutorSocketPath string            `json:"executor_socket_path"`
	OPMSExtraHeaders   map[string]string `json:"opms_extra_headers"`
	OPMSProxyURL       string            `json:"opms_proxy_url"`
	SkipSSLValidation  bool              `json:"skip_ssl_validation"`
	MinTLSVersion      string            `json:"min_tls_version"`
}

// Commands returns the bootstrap-par-control subcommand.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap-par-control",
		Short: "Bootstrap the PAR split-mode control plane",
		Long:  "Provides config that par-control needs to boot.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(run,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath)),
					LogParams:    log.ForOneShot(command.LoggerName, "off", false),
				}),
				core.Bundle(core.WithSecrets()),
				hostnameimpl.Module(),
			)
		},
	}
	return []*cobra.Command{cmd}
}

func run(cfg config.Component, hostnameComp hostname.Component) error {
	return bootstrap(context.Background(), cfg, hostnameComp, enrollAndPersist, os.Stdout)
}

func bootstrap(ctx context.Context, cfg config.Component, hostnameComp hostname.Component, enrollAndPersist enrollAndPersistFunc, out io.Writer) error {
	resolved, err := resolveConfig(ctx, cfg, hostnameComp, enrollAndPersist)
	if err != nil {
		return err
	}
	return emitConfig(out, resolved)
}

func resolveConfig(ctx context.Context, cfg config.Component, hostnameComp hostname.Component, enrollAndPersist enrollAndPersistFunc) (*ControlPlaneConfig, error) {
	logLevel := cfg.GetString("log_level")
	splitMode := cfg.GetBool(par.PAREnabled) && cfg.GetBool(par.PARSplitEnabled)

	if !splitMode {
		return &ControlPlaneConfig{SplitMode: false, LogLevel: logLevel}, nil
	}

	if err := rejectFIPS(cfg); err != nil {
		return nil, err
	}

	if err := ensureEnrollment(ctx, cfg, hostnameComp, enrollAndPersist); err != nil {
		return nil, err
	}

	cert.PersistCertFilepath(cfg)

	urn := cfg.GetString(par.PARUrn)
	privateKey := cfg.GetString(par.PARPrivateKey)
	if urn == "" || privateKey == "" {
		return nil, errors.New("the resolved Private Action Runner identity is incomplete")
	}
	identity, err := parutil.ParseRunnerURN(urn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the Private Action Runner identity: %w", err)
	}

	runtime, err := resolveRuntimeConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &ControlPlaneConfig{
		SplitMode: true,
		LogLevel:  logLevel,
		Runtime:   runtime,
		Identity: &Identity{
			URN:        urn,
			PrivateKey: privateKey,
			OrgID:      identity.OrgID,
			RunnerID:   identity.RunnerID,
		},
		IPCCertFilePath: cfg.GetString("ipc_cert_file_path"),
	}, nil
}

func resolveRuntimeConfig(cfg config.Component) (*RuntimeConfig, error) {
	parCfg, err := parconfig.FromDDConfig(cfg, nil)
	if err != nil {
		return nil, err
	}
	endpoint := opms.EndpointURL(parCfg, "")
	request, err := http.NewRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, errors.New("invalid PAR OPMS endpoint")
	}
	proxyURL := ""
	if proxies := cfg.GetProxies(); proxies != nil {
		proxy, err := httputils.GetProxyTransportFunc(proxies, cfg)(request)
		if err != nil {
			return nil, errors.New("failed to resolve PAR OPMS proxy")
		}
		if proxy != nil {
			proxyURL = proxy.String()
		}
	}
	return &RuntimeConfig{
		OPMSBaseURL:        endpoint,
		TaskConcurrency:    parCfg.RunnerPoolSize,
		ExecutorSocketPath: cfg.GetString(par.PARExecutorSocketPath),
		OPMSExtraHeaders:   parCfg.OpmsExtraHeaders,
		OPMSProxyURL:       proxyURL,
		SkipSSLValidation:  cfg.GetBool("skip_ssl_validation"),
		MinTLSVersion:      cfg.GetString("min_tls_version"),
	}, nil
}

func rejectFIPS(cfg config.Component) error {
	if cfg.GetBool("fips.enabled") {
		return errors.New("private_action_runner.split_enabled is not supported with fips.enabled; disable split mode or FIPS mode")
	}
	if enabled, err := fips.Enabled(); err == nil && enabled {
		return errors.New("private_action_runner.split_enabled is not supported by the FIPS Agent; use the monolithic runner")
	}
	return nil
}

func emitConfig(out io.Writer, resolved *ControlPlaneConfig) error {
	encoded, err := json.Marshal(resolved)
	if err != nil {
		return errors.New("failed to serialize the par-control configuration")
	}
	if _, err := out.Write(encoded); err != nil {
		return errors.New("failed to write the par-control configuration")
	}
	return nil
}
