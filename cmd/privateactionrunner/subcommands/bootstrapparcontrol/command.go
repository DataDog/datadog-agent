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
	parutil "github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
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

	Identity *Identity `json:"identity,omitempty"`

	AgentVersion      string `json:"agent_version,omitempty"`
	CmdPort           int    `json:"cmd_port,omitempty"`
	AuthTokenFilePath string `json:"auth_token_file_path,omitempty"`
	IPCCertFilePath   string `json:"ipc_cert_file_path,omitempty"`
}

// Commands returns the bootstrap-par-control subcommand.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap-par-control",
		Short: "Bootstrap the Private Action Runner split-mode control plane",
		Long: `Loads the canonical Agent configuration, ensures that the runner has a valid
identity, and writes the identity and Core Agent IPC bootstrap settings to stdout.
Runtime configuration is consumed directly from the Core Agent configuration stream.

When split mode is disabled the command succeeds without enrolling and reports
only the launch gate and log level.`,
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

	return &ControlPlaneConfig{
		SplitMode: true,
		LogLevel:  logLevel,
		Identity: &Identity{
			URN:        urn,
			PrivateKey: privateKey,
			OrgID:      identity.OrgID,
			RunnerID:   identity.RunnerID,
		},
		AgentVersion:      version.AgentVersion,
		CmdPort:           cfg.GetInt("cmd_port"),
		AuthTokenFilePath: cfg.GetString("auth_token_file_path"),
		IPCCertFilePath:   cfg.GetString("ipc_cert_file_path"),
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
