// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configstreambootstrap applies the initial configstream snapshot to
// the global config builder before FX starts, so constructor-time config reads
// see streamed values rather than local datadog.yaml. Callers must follow up
// with config.WithConfigstreamEnabled(true) so FX does not re-read the file
// and clobber the snapshot.
package configstreambootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	yaml "go.yaml.in/yaml/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgtoken "github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// EnabledEnvVar gates whether the bootstrap runs.
	EnabledEnvVar = "DD_REMOTE_AGENT_CONFIGSTREAM_CONSUMER_ENABLED"

	envAuthTokenFilePath  = "DD_AUTH_TOKEN_FILE_PATH"
	envIPCCertFilePath    = "DD_IPC_CERT_FILE_PATH"
	envCmdHost            = "DD_CMD_HOST"
	envCmdPort            = "DD_CMD_PORT"
	envRARRegistryEnabled = "DD_REMOTE_AGENT_REGISTRY_ENABLED"

	defaultCmdHost = "localhost"
	defaultCmdPort = 5001

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
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return false
		}
		return cfg.RemoteAgent.ConfigStream.Consumer.Enabled
	}
	return false
}

type settings struct {
	AuthTokenFilePath  string
	IPCCertFilePath    string
	CmdHost            string
	CmdPort            int
	RARRegistryEnabled bool
}

// readSettings resolves bootstrap values from env, then a YAML parse bounded
// to a small set of keys (so malformed sections elsewhere don't abort).
func readSettings(cliConfigPath string, lookupEnv func(string) (string, bool)) settings {
	bs := settings{CmdHost: defaultCmdHost, CmdPort: defaultCmdPort}

	authEnv, hasAuthEnv := lookupEnv(envAuthTokenFilePath)
	certEnv, hasCertEnv := lookupEnv(envIPCCertFilePath)
	hostEnv, hasHostEnv := lookupEnv(envCmdHost)
	portEnv, hasPortEnv := lookupEnv(envCmdPort)
	rarEnv, hasRAREnv := lookupEnv(envRARRegistryEnabled)

	if hasAuthEnv && authEnv != "" {
		bs.AuthTokenFilePath = authEnv
	}
	if hasCertEnv && certEnv != "" {
		bs.IPCCertFilePath = certEnv
	}
	if hasHostEnv && hostEnv != "" {
		bs.CmdHost = hostEnv
	}
	if hasPortEnv {
		if p, err := strconv.Atoi(portEnv); err == nil && p > 0 {
			bs.CmdPort = p
		}
	}
	if hasRAREnv {
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
			RemoteAgent       struct {
				Registry struct {
					Enabled bool `yaml:"enabled"`
				} `yaml:"registry"`
			} `yaml:"remote_agent"`
		}
		_ = yaml.Unmarshal(data, &cfg)

		if !hasAuthEnv && cfg.AuthTokenFilePath != "" {
			bs.AuthTokenFilePath = cfg.AuthTokenFilePath
		}
		if !hasCertEnv && cfg.IPCCertFilePath != "" {
			bs.IPCCertFilePath = cfg.IPCCertFilePath
		}
		if !hasHostEnv && cfg.CmdHost != "" {
			bs.CmdHost = cfg.CmdHost
		}
		if !hasPortEnv && cfg.CmdPort > 0 {
			bs.CmdPort = cfg.CmdPort
		}
		if !hasRAREnv {
			bs.RARRegistryEnabled = cfg.RemoteAgent.Registry.Enabled
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

func seedGlobalBuilder(bs settings) {
	b := pkgconfigsetup.GlobalConfigBuilder()
	if bs.AuthTokenFilePath != "" {
		b.Set("auth_token_file_path", bs.AuthTokenFilePath, pkgconfigmodel.SourceFile)
	}
	if bs.IPCCertFilePath != "" {
		b.Set("ipc_cert_file_path", bs.IPCCertFilePath, pkgconfigmodel.SourceFile)
	}
	b.Set("cmd_host", bs.CmdHost, pkgconfigmodel.SourceFile)
	b.Set("cmd_port", bs.CmdPort, pkgconfigmodel.SourceFile)
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
	seedGlobalBuilder(bs)

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

	// Listener exists only so RAR has a syntactically valid ApiEndpointUri;
	// we don't serve on it. The FX-side remoteagent component owns the
	// long-lived listener and re-registers with its own URI.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("open bootstrap listener: %w", err)
	}
	defer listener.Close()

	addr := net.JoinHostPort(bs.CmdHost, strconv.Itoa(bs.CmdPort))
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)),
		grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(authToken)),
	)
	if err != nil {
		return fmt.Errorf("dial core agent at %s: %w", addr, err)
	}
	defer conn.Close()

	client := pbcore.NewAgentSecureClient(conn)

	pkglog.Infof("configstream bootstrap[%s]: registering with RAR at %s", params.ClientName, addr)
	sessionID, err := registerForBootstrap(ctx, client, params.ClientName, listener.Addr().String())
	if err != nil {
		return fmt.Errorf("register with RAR: %w", err)
	}

	pkglog.Infof("configstream bootstrap[%s]: requesting initial snapshot", params.ClientName)
	if err := fetchAndApplySnapshot(ctx, client, params.ClientName, sessionID); err != nil {
		return fmt.Errorf("fetch initial snapshot: %w", err)
	}
	pkglog.Infof("configstream bootstrap[%s]: snapshot applied", params.ClientName)
	return nil
}

func registerForBootstrap(ctx context.Context, client pbcore.AgentSecureClient, clientName, apiEndpointURI string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	resp, err := client.RegisterRemoteAgent(ctx, &pbcore.RegisterRemoteAgentRequest{
		Flavor:         flavor.GetFlavor(),
		DisplayName:    clientName + " (bootstrap)",
		ApiEndpointUri: apiEndpointURI,
		Services:       nil,
	})
	if err != nil {
		return "", err
	}
	return resp.SessionId, nil
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
func pbValueToGo(v *structpb.Value) any {
	if v == nil {
		return nil
	}
	result := v.AsInterface()
	if f, ok := result.(float64); ok {
		if f == float64(int64(f)) {
			return int64(f)
		}
	}
	return result
}
