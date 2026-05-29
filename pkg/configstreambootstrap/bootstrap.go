// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configstreambootstrap applies the initial configstream snapshot to
// the global config builder before FX starts. Callers must follow up with
// config.WithConfigstreamEnabled(true) so FX does not re-read datadog.yaml
// and clobber the snapshot.
//
// Intentionally minimal: setupConfig features NOT applied here — secret
// resolution, fleet policy merging, DD_COMMON_ROOT, delegated auth. Bootstrap
// keys must be plain values in env or datadog.yaml.
package configstreambootstrap

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"
	yaml "go.yaml.in/yaml/v3"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/remoteagent/helper"
	pkgtoken "github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// EnabledEnvVar gates whether the bootstrap runs.
	EnabledEnvVar = "DD_REMOTE_AGENT_CONFIGSTREAM_CONSUMER_ENABLED"

	envAuthTokenFilePath  = "DD_AUTH_TOKEN_FILE_PATH"
	envIPCCertFilePath    = "DD_IPC_CERT_FILE_PATH"
	envCmdHost            = "DD_CMD_HOST"
	envCmdPort            = "DD_CMD_PORT"
	envVSockAddr          = "DD_VSOCK_ADDR"
	envRARRegistryEnabled = "DD_REMOTE_AGENT_REGISTRY_ENABLED"

	defaultCmdHost            = "localhost"
	defaultCmdPort            = 5001
	defaultRARRegistryEnabled = true

	// queryTimeout caps RegisterRemoteAgent and stream open. Snapshot Recv()
	// uses the caller's ctx and is not capped.
	queryTimeout = 30 * time.Second
)

// Params identifies the remote agent calling the bootstrap.
type Params struct {
	ClientName    string
	CLIConfigPath string
	LookupEnv     func(string) (string, bool)
}

// IsEnabled returns the configstream consumer flag from env or datadog.yaml.
// A YAML parse error returns false; users with malformed datadog.yaml should
// set DD_REMOTE_AGENT_CONFIGSTREAM_CONSUMER_ENABLED so the env path wins.
func IsEnabled(cliConfigPath string, lookupEnv func(string) (string, bool)) bool {
	if v, ok := lookupEnv(EnabledEnvVar); ok {
		if enabled, err := strconv.ParseBool(v); err == nil {
			return enabled
		}
	}
	for _, path := range yamlCandidates(cliConfigPath) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg struct {
			RemoteAgent struct {
				ConfigStream struct {
					Consumer struct {
						Enabled bool `yaml:"enabled"`
					} `yaml:"consumer"`
				} `yaml:"configstream"`
			} `yaml:"remote_agent"`
		}
		_ = yaml.Unmarshal(data, &cfg)
		return cfg.RemoteAgent.ConfigStream.Consumer.Enabled
	}
	return false
}

type settings struct {
	AuthTokenFilePath  string
	IPCCertFilePath    string
	CmdHost            string
	CmdPort            int
	VSockAddr          string
	RARRegistryEnabled bool
}

// readSettings resolves bootstrap values from env, then a YAML parse bounded
// to a small set of keys (so malformed sections elsewhere don't abort).
// Empty env values are treated as unset, so YAML fallback still applies.
func readSettings(cliConfigPath string, lookupEnv func(string) (string, bool)) settings {
	bs := settings{CmdHost: defaultCmdHost, CmdPort: defaultCmdPort, RARRegistryEnabled: defaultRARRegistryEnabled}

	authEnv, _ := lookupEnv(envAuthTokenFilePath)
	certEnv, _ := lookupEnv(envIPCCertFilePath)
	hostEnv, _ := lookupEnv(envCmdHost)
	portEnv, _ := lookupEnv(envCmdPort)
	vsockEnv, _ := lookupEnv(envVSockAddr)
	rarEnv, _ := lookupEnv(envRARRegistryEnabled)

	if authEnv != "" {
		bs.AuthTokenFilePath = authEnv
	}
	if certEnv != "" {
		bs.IPCCertFilePath = certEnv
	}
	if hostEnv != "" {
		bs.CmdHost = hostEnv
	}
	if portEnv != "" {
		if p, err := strconv.Atoi(portEnv); err == nil && p > 0 {
			bs.CmdPort = p
		}
	}
	if vsockEnv != "" {
		bs.VSockAddr = vsockEnv
	}
	if rarEnv != "" {
		if v, err := strconv.ParseBool(rarEnv); err == nil {
			bs.RARRegistryEnabled = v
		}
	}

	for _, path := range yamlCandidates(cliConfigPath) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg struct {
			AuthTokenFilePath string `yaml:"auth_token_file_path"`
			IPCCertFilePath   string `yaml:"ipc_cert_file_path"`
			CmdHost           string `yaml:"cmd_host"`
			CmdPort           int    `yaml:"cmd_port"`
			VSockAddr         string `yaml:"vsock_addr"`
			RemoteAgent       struct {
				Registry struct {
					// *bool to distinguish absent from explicit false.
					Enabled *bool `yaml:"enabled"`
				} `yaml:"registry"`
			} `yaml:"remote_agent"`
		}
		_ = yaml.Unmarshal(data, &cfg)

		if authEnv == "" && cfg.AuthTokenFilePath != "" {
			bs.AuthTokenFilePath = cfg.AuthTokenFilePath
		}
		if certEnv == "" && cfg.IPCCertFilePath != "" {
			bs.IPCCertFilePath = cfg.IPCCertFilePath
		}
		if hostEnv == "" && cfg.CmdHost != "" {
			bs.CmdHost = cfg.CmdHost
		}
		if portEnv == "" && cfg.CmdPort > 0 {
			bs.CmdPort = cfg.CmdPort
		}
		if vsockEnv == "" && cfg.VSockAddr != "" {
			bs.VSockAddr = cfg.VSockAddr
		}
		if rarEnv == "" && cfg.RemoteAgent.Registry.Enabled != nil {
			bs.RARRegistryEnabled = *cfg.RemoteAgent.Registry.Enabled
		}
		break
	}
	return bs
}

func yamlCandidates(cliConfigPath string) []string {
	out := make([]string, 0, 2)
	for _, path := range []string{cliConfigPath, config.DefaultConfPath} {
		if path == "" {
			continue
		}
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			path = filepath.Join(path, "datadog.yaml")
		}
		out = append(out, path)
	}
	return out
}

func seedGlobalBuilder(bs settings, cliConfigPath string) {
	b := pkgconfigsetup.GlobalConfigBuilder()
	// Lets the IPC primitives' filepath.Dir(ConfigFileUsed()) fallback resolve.
	if candidates := yamlCandidates(cliConfigPath); len(candidates) > 0 {
		b.SetConfigFile(candidates[0])
	}
	if bs.AuthTokenFilePath != "" {
		b.Set("auth_token_file_path", bs.AuthTokenFilePath, pkgconfigmodel.SourceFile)
	}
	if bs.IPCCertFilePath != "" {
		b.Set("ipc_cert_file_path", bs.IPCCertFilePath, pkgconfigmodel.SourceFile)
	}
	b.Set("cmd_host", bs.CmdHost, pkgconfigmodel.SourceFile)
	b.Set("cmd_port", bs.CmdPort, pkgconfigmodel.SourceFile)
	if bs.VSockAddr != "" {
		b.Set("vsock_addr", bs.VSockAddr, pkgconfigmodel.SourceFile)
	}
	b.Set("remote_agent.registry.enabled", bs.RARRegistryEnabled, pkgconfigmodel.SourceFile)
}

// Run dials the core agent, registers with RAR, and applies the initial
// snapshot. Blocks indefinitely on connection failure per the RFC's no-fallback
// guidance; cancel via ctx. Callers must follow up with config.NewParams (not
// NewAgentParams) so they do not re-init the global and wipe the snapshot.
func Run(ctx context.Context, params Params) error {
	if params.ClientName == "" {
		return errors.New("bootstrap: ClientName is required")
	}
	if params.LookupEnv == nil {
		params.LookupEnv = os.LookupEnv
	}

	bs := readSettings(params.CLIConfigPath, params.LookupEnv)

	if !bs.RARRegistryEnabled {
		return fmt.Errorf("configstream consumer requires remote_agent.registry.enabled=true; refusing to start %s without RAR", params.ClientName)
	}

	pkgconfigsetup.InitConfigObjects(params.CLIConfigPath, config.DefaultConfPath)
	seedGlobalBuilder(bs, params.CLIConfigPath)

	reader := pkgconfigsetup.GlobalConfigBuilder()

	// FetchOrCreate* are idempotent against the on-disk artifacts, so the
	// FX-side ipcfx.ModuleReadWrite constructor can safely re-run them later.
	authToken, err := pkgtoken.FetchOrCreateAuthToken(ctx, reader)
	if err != nil {
		return fmt.Errorf("fetch auth token: %w", err)
	}
	clientTLS, _, _, err := cert.FetchOrCreateIPCCert(ctx, reader)
	if err != nil {
		return fmt.Errorf("fetch IPC cert: %w", err)
	}

	addr := net.JoinHostPort(bs.CmdHost, strconv.Itoa(bs.CmdPort))
	logger := pkglog.NewWrapper(2)
	vsockAddr := reader.GetString("vsock_addr")

	// Clear the local env layer before applying the snapshot so streamed SourceEnvVar values
	// from the core agent land in an empty c.envs.
	disableLocalEnvLayer(params.ClientName)

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 500 * time.Millisecond
	bo.MaxInterval = time.Minute
	bo.Reset()
	for attempt := 1; ; attempt++ {
		err := tryBootstrap(ctx, params.ClientName, addr, authToken, clientTLS, vsockAddr, logger)
		if err == nil {
			pkglog.Infof("configstream bootstrap[%s]: snapshot applied", params.ClientName)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		next := bo.NextBackOff()
		if next == backoff.Stop || next < 0 {
			// avoid hot loop on Stop sentinel
			next = bo.MaxInterval
		}
		pkglog.Warnf("configstream bootstrap[%s]: attempt %d failed (%v); retrying in %s", params.ClientName, attempt, err, next)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(next):
		}
	}
}

// disableLocalEnvLayer drops the env layer on configs that support it (nodetreemodel only).
func disableLocalEnvLayer(clientName string) {
	type envLayerSkipper interface{ SkipEnvLayer() }
	if skipper, ok := pkgconfigsetup.GlobalConfigBuilder().(envLayerSkipper); ok {
		skipper.SkipEnvLayer()
		pkglog.Infof("configstream bootstrap[%s]: local env-var layer disabled; core-agent values are the single source of truth", clientName)
		return
	}
	pkglog.Warnf("configstream bootstrap[%s]: config impl does not support disabling the local env-var layer; subprocess env vars may override streamed values", clientName)
}

// tryBootstrap performs one dial → register → fetch-snapshot attempt. A fresh
// listener is allocated per call so a stale port doesn't survive retries.
func tryBootstrap(ctx context.Context, clientName, addr, authToken string, clientTLS *tls.Config, vsockAddr string, logger log.Component) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("open bootstrap listener: %w", err)
	}
	defer listener.Close()

	client, conn, err := helper.NewAgentSecureClient(addr, authToken, clientTLS, vsockAddr, logger)
	if err != nil {
		return fmt.Errorf("dial core agent at %s: %w", addr, err)
	}
	defer conn.Close()

	sessionID, _, err := helper.RegisterRemoteAgent(ctx, client, helper.RegistrationRequest{
		Flavor:         flavor.GetFlavor(),
		DisplayName:    clientName + " (bootstrap)",
		APIEndpointURI: "https://" + listener.Addr().String(),
	}, queryTimeout, 0, logger)
	if err != nil {
		return fmt.Errorf("register with RAR: %w", err)
	}

	pkglog.Infof("configstream bootstrap[%s]: requesting initial snapshot", clientName)
	if err := fetchAndApplySnapshot(ctx, client, clientName, sessionID); err != nil {
		return fmt.Errorf("fetch initial snapshot: %w", err)
	}
	return nil
}

func fetchAndApplySnapshot(ctx context.Context, client pbcore.AgentSecureClient, clientName, sessionID string) error {
	md := metadata.New(map[string]string{"session_id": sessionID})
	streamCtx := metadata.NewOutgoingContext(ctx, md)

	stream, err := client.StreamConfigEvents(streamCtx, &pbcore.ConfigStreamRequest{Name: clientName})
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}

	for {
		ev, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("stream recv: %w", err)
		}
		snap, ok := ev.Event.(*pbcore.ConfigEvent_Snapshot)
		if !ok {
			// Producer guarantees snapshot-first; skip anything that races in.
			continue
		}
		applySnapshotToGlobalBuilder(clientName, snap.Snapshot)
		return nil
	}
}

func applySnapshotToGlobalBuilder(clientName string, snapshot *pbcore.ConfigSnapshot) {
	b := pkgconfigsetup.GlobalConfigBuilder()
	for _, setting := range snapshot.Settings {
		b.Set(setting.Key, pbValueToGo(setting.Value), pkgconfigmodel.Source(setting.Source))
	}
	pkglog.Infof("configstream bootstrap[%s]: applied snapshot seq_id=%d settings=%d", clientName, snapshot.SequenceId, len(snapshot.Settings))
}

// pbValueToGo preserves integer-typed values that structpb has widened to float64.
// Bounded to |x| <= 2^53 — beyond that, float64 can't represent consecutive integers.
func pbValueToGo(v *structpb.Value) any {
	if v == nil {
		return nil
	}
	result := v.AsInterface()
	if f, ok := result.(float64); ok {
		const maxExactInt = 1 << 53
		if !math.IsNaN(f) && !math.IsInf(f, 0) && f >= -maxExactInt && f <= maxExactInt && f == float64(int64(f)) {
			return int64(f)
		}
	}
	return result
}
