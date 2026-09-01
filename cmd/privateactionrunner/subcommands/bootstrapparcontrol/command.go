// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package bootstrapparcontrol implements the 'bootstrap-par-control' subcommand
// for the private-action-runner. It is the single configuration authority for
// split mode: it loads the canonical Agent configuration, ensures the runner is
// enrolled, and emits the resolved control-plane configuration for par-control.
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
	identitycmd "github.com/DataDog/datadog-agent/cmd/privateactionrunner/subcommands/identity"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	par "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/fips"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// ConfigPathEnv names the file created by par-control for the resolved
// configuration. Keeping the payload off stdout prevents secrets from being
// mixed with bootstrap logs.
const ConfigPathEnv = "DD_PAR_CONTROL_CONFIG_PATH"

// Identity is the runner identity par-control signs OPMS requests with.
type Identity struct {
	URN        string `json:"urn"`
	PrivateKey string `json:"private_key"`
	OrgID      int64  `json:"org_id"`
	RunnerID   string `json:"runner_id"`
}

// TLS mirrors the Agent TLS settings par-control applies to OPMS connections.
type TLS struct {
	SkipSSLValidation bool   `json:"skip_ssl_validation"`
	MinTLSVersion     string `json:"min_tls_version"`
}

// ControlPlaneConfig is everything par-control needs to run. Go is the single
// authority for these values; par-control only validates them at its trust
// boundary.
//
// Durations carry explicit units instead of relying on Go or Rust duration
// serialization.
type ControlPlaneConfig struct {
	// SplitMode gates the whole control plane. When false, the monolithic
	// runner owns OPMS polling and par-control exits successfully.
	SplitMode bool   `json:"split_mode"`
	LogLevel  string `json:"log_level"`

	// Pointers because encoding/json ignores omitempty on structs, and the
	// gate-only response should not carry empty identity or transport objects.
	Identity *Identity `json:"identity,omitempty"`

	OPMSBaseURL      string            `json:"opms_base_url,omitempty"`
	OPMSProxyURL     string            `json:"opms_proxy_url,omitempty"`
	AgentVersion     string            `json:"agent_version,omitempty"`
	Modes            []string          `json:"modes,omitempty"`
	TaskConcurrency  int32             `json:"task_concurrency,omitempty"`
	ExecutorSocket   string            `json:"executor_socket,omitempty"`
	IPCCertFilePath  string            `json:"ipc_cert_file_path,omitempty"`
	OPMSExtraHeaders map[string]string `json:"opms_extra_headers,omitempty"`

	TLS *TLS `json:"tls,omitempty"`

	LoopIntervalMilliseconds       int64 `json:"loop_interval_milliseconds,omitempty"`
	HeartbeatIntervalMilliseconds  int64 `json:"heartbeat_interval_milliseconds,omitempty"`
	HealthCheckIntervalMillisecond int64 `json:"health_check_interval_milliseconds,omitempty"`
	OPMSRequestTimeoutMilliseconds int64 `json:"opms_request_timeout_milliseconds,omitempty"`
	MinBackoffMilliseconds         int64 `json:"min_backoff_milliseconds,omitempty"`
	MaxBackoffMilliseconds         int64 `json:"max_backoff_milliseconds,omitempty"`
	WaitBeforeRetryMilliseconds    int64 `json:"wait_before_retry_milliseconds,omitempty"`
	MaxAttempts                    int32 `json:"max_attempts,omitempty"`
}

// Commands returns the bootstrap-par-control subcommand.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap-par-control",
		Short: "Resolve the Private Action Runner split-mode control-plane configuration",
		Long: `Loads the canonical Agent configuration, ensures that the runner has a valid
identity, and writes the resolved par-control configuration to the file named
by DD_PAR_CONTROL_CONFIG_PATH.

When split mode is disabled the command succeeds without enrolling and reports
only the launch gate and log level.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(run,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath)),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true),
				}),
				core.Bundle(core.WithSecrets()),
				hostnameimpl.Module(),
			)
		},
	}
	return []*cobra.Command{cmd}
}

func run(logger log.Component, cfg config.Component, hostnameComp hostname.Component) error {
	path := os.Getenv(ConfigPathEnv)
	if path == "" {
		return fmt.Errorf("%s is not set", ConfigPathEnv)
	}
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return errors.New("failed to open the par-control configuration file")
	}
	defer out.Close()
	return bootstrap(context.Background(), logger, cfg, hostnameComp, identitycmd.EnrollAndPersist, out)
}

func bootstrap(ctx context.Context, logger log.Component, cfg config.Component, hostnameComp hostname.Component, enrollAndPersist enrollAndPersistFunc, out io.Writer) error {
	resolved, err := resolveConfig(ctx, logger, cfg, hostnameComp, enrollAndPersist)
	if err != nil {
		return err
	}
	return emitConfig(out, resolved)
}

func resolveConfig(ctx context.Context, logger log.Component, cfg config.Component, hostnameComp hostname.Component, enrollAndPersist enrollAndPersistFunc) (*ControlPlaneConfig, error) {
	logLevel := cfg.GetString("log_level")
	splitMode := cfg.GetBool(par.PAREnabled) && cfg.GetBool(par.PARSplitEnabled)

	// Nothing below is meaningful for the monolithic runner, and none of it
	// should run: enrollment in particular has side effects.
	if !splitMode {
		return &ControlPlaneConfig{SplitMode: false, LogLevel: logLevel}, nil
	}

	if err := rejectFIPS(cfg); err != nil {
		return nil, err
	}

	if err := ensureEnrollment(ctx, logger, cfg, hostnameComp, enrollAndPersist); err != nil {
		return nil, err
	}

	// Enrollment may have just written an identity, and a persisted identity
	// wins over inline configuration.
	if err := applyPersistedIdentity(ctx, cfg); err != nil {
		return nil, err
	}

	// A concrete path is required: par-control has no Agent config of its own.
	cert.PersistCertFilepath(cfg)

	parCfg, err := parconfig.FromDDConfig(cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to derive the Private Action Runner configuration: %w", err)
	}
	if parCfg.IdentityIsIncomplete() {
		return nil, errors.New("the resolved Private Action Runner identity is incomplete")
	}

	opmsBaseURL := parCfg.OPMSEndpointURL("")
	opmsProxyURL, err := resolveProxy(cfg, opmsBaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the OPMS proxy: %w", err)
	}

	return &ControlPlaneConfig{
		SplitMode: true,
		LogLevel:  logLevel,
		Identity: &Identity{
			URN:        parCfg.Urn,
			PrivateKey: cfg.GetString(par.PARPrivateKey),
			OrgID:      parCfg.OrgId,
			RunnerID:   parCfg.RunnerId,
		},
		OPMSBaseURL:      opmsBaseURL,
		OPMSProxyURL:     opmsProxyURL,
		AgentVersion:     parCfg.Version,
		Modes:            modes.ToStrings(parCfg.Modes),
		TaskConcurrency:  parCfg.RunnerPoolSize,
		ExecutorSocket:   cfg.GetString(par.PARExecutorSocketPath),
		IPCCertFilePath:  cfg.GetString("ipc_cert_file_path"),
		OPMSExtraHeaders: parCfg.OpmsExtraHeaders,
		TLS: &TLS{
			SkipSSLValidation: cfg.GetBool("skip_ssl_validation"),
			MinTLSVersion:     cfg.GetString("min_tls_version"),
		},
		LoopIntervalMilliseconds:       parCfg.LoopInterval.Milliseconds(),
		HeartbeatIntervalMilliseconds:  parCfg.HeartbeatInterval.Milliseconds(),
		HealthCheckIntervalMillisecond: int64(parCfg.HealthCheckInterval),
		OPMSRequestTimeoutMilliseconds: int64(parCfg.OpmsRequestTimeout),
		MinBackoffMilliseconds:         parCfg.MinBackoff.Milliseconds(),
		MaxBackoffMilliseconds:         parCfg.MaxBackoff.Milliseconds(),
		WaitBeforeRetryMilliseconds:    parCfg.WaitBeforeRetry.Milliseconds(),
		MaxAttempts:                    parCfg.MaxAttempts,
	}, nil
}

// rejectFIPS refuses to bootstrap split mode in FIPS mode. The Agent config
// loader applies FIPS endpoint transforms, so continuing would silently consume
// them and imply FIPS support that split mode has neither designed nor
// validated. This is not a Gov-site restriction: Gov sites work in split mode as
// long as FIPS mode is off.
func rejectFIPS(cfg config.Component) error {
	if cfg.GetBool("fips.enabled") {
		return errors.New("private_action_runner.split_enabled is not supported with fips.enabled; disable split mode or FIPS mode")
	}
	if enabled, err := fips.Enabled(); err == nil && enabled {
		return errors.New("private_action_runner.split_enabled is not supported by the FIPS Agent; use the monolithic runner")
	}
	return nil
}

// applyPersistedIdentity makes the persisted identity the effective one so that
// FromDDConfig derives the URN, org ID, and runner ID from it. Inline identity
// remains the fallback when nothing is persisted.
func applyPersistedIdentity(ctx context.Context, cfg config.Component) error {
	persisted, err := enrollment.GetIdentityFromPreviousEnrollment(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to load persisted identity: %w", err)
	}
	if persisted == nil {
		return nil
	}
	cfg.Set(par.PARUrn, persisted.URN, model.SourceAgentRuntime)
	cfg.Set(par.PARPrivateKey, persisted.PrivateKey, model.SourceAgentRuntime)
	return nil
}

func resolveProxy(cfg config.Component, target string) (string, error) {
	proxies := cfg.GetProxies()
	if proxies == nil {
		return "", nil
	}

	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return "", errors.New("invalid OPMS URL")
	}
	proxyURL, err := httputils.GetProxyTransportFunc(proxies, cfg)(request)
	if err != nil {
		return "", errors.New("invalid Agent proxy configuration")
	}
	if proxyURL == nil {
		return "", nil
	}
	return proxyURL.String(), nil
}

// emitConfig writes the configuration as compact JSON. The payload carries the
// runner private key and may carry proxy credentials, so errors never include it.
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
