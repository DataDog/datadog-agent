// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package live

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.uber.org/fx"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/output"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	confighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	probeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	probeconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// Only connection and authentication locations are consumed from installed
// configuration. In particular, intake destinations, credentials, check enable
// flags, direct-send settings, and collection intervals are never applied.
type connectionSettings struct {
	Host      *string `yaml:"cmd_host"`
	Legacy    *string `yaml:"ipc_address"`
	Port      *int    `yaml:"cmd_port"`
	TokenFile *string `yaml:"auth_token_file_path"`
	CertFile  *string `yaml:"ipc_cert_file_path"`
	Process   struct {
		Port *int `yaml:"cmd_port"`
	} `yaml:"process_config"`
	Probe struct {
		Socket *string `yaml:"sysprobe_socket"`
	} `yaml:"system_probe_config"`
}

type connectionConfig struct {
	config.Component
	path string
}

func (c *connectionConfig) ConfigFileUsed() string { return c.path }

var errInstalledConfig = errors.New("cannot read installed Agent API configuration; check capture --cfgpath")

func readConnectionSettings(path string) (connectionSettings, error) {
	file, err := os.Open(path)
	if err != nil {
		return connectionSettings{}, err
	}
	defer file.Close()
	bounded := &io.LimitedReader{R: file, N: 4<<20 + 1}
	decoder := yaml.NewDecoder(bounded)
	var settings connectionSettings
	if decoder.Decode(&settings) != nil || decoder.Decode(new(any)) != io.EOF || bounded.N <= 0 {
		return connectionSettings{}, errInstalledConfig
	}
	return settings, nil
}

func installedConfig(cfgpath string, includeProbe bool) (config.Component, error) {
	if cfgpath == "" {
		cfgpath = config.DefaultConfPath
	}
	info, err := os.Stat(cfgpath)
	if err != nil {
		return nil, errInstalledConfig
	}
	if info.IsDir() {
		cfgpath = filepath.Join(cfgpath, "datadog.yaml")
	}
	path, err := filepath.Abs(cfgpath)
	if err != nil {
		return nil, errInstalledConfig
	}
	settings, err := readConnectionSettings(path)
	if err != nil {
		return nil, errInstalledConfig
	}
	cfg := &connectionConfig{Component: output.NewConfig(), path: path}
	stringsToCopy := map[string]*string{
		"cmd_host": settings.Host, "ipc_address": settings.Legacy,
		"auth_token_file_path": settings.TokenFile, "ipc_cert_file_path": settings.CertFile,
		"system_probe_config.sysprobe_socket": settings.Probe.Socket,
	}
	for key, value := range stringsToCopy {
		if value != nil {
			cfg.Set(key, *value, configmodel.SourceFile)
		}
	}
	for key, value := range map[string]*int{"cmd_port": settings.Port, "process_config.cmd_port": settings.Process.Port} {
		if value != nil {
			cfg.Set(key, *value, configmodel.SourceFile)
		}
	}
	if includeProbe {
		// The installed system-probe's configuration is adjacent to datadog.yaml.
		// Only its socket address is used; discovery determines its actual owner.
		probe, err := readConnectionSettings(filepath.Join(filepath.Dir(path), "system-probe.yaml"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, errInstalledConfig
		}
		if probe.Probe.Socket != nil {
			cfg.Set("system_probe_config.sysprobe_socket", *probe.Probe.Socket, configmodel.SourceFile)
		}
	}
	return cfg, nil
}

func installedClients(cfg config.Component, auth ipc.Component, includeProbe bool) ([]Client, func(), error) {
	address, err := confighelper.GetIPCAddress(cfg)
	if err != nil {
		return nil, nil, errors.New("capture requires local Agent API addresses")
	}
	port := cfg.GetInt("cmd_port")
	if port <= 0 || port > 65535 {
		return nil, nil, errors.New("invalid core Agent API port")
	}
	processAddress, err := confighelper.GetProcessAPIAddressPort(cfg)
	if err != nil || cfg.GetInt("process_config.cmd_port") > 65535 {
		return nil, nil, errors.New("invalid Process Agent API address")
	}
	// Use the existing IPC trust and token with a bounded response adapter. The
	// general IPC client's Do helper reads response bodies without a byte limit.
	transport := &http.Transport{
		TLSClientConfig: auth.GetTLSClientConfig(), DisableCompression: true,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second,
		IdleConnTimeout: 30 * time.Second, MaxConnsPerHost: 4,
	}
	client := &http.Client{Transport: transport}
	clients := []Client{
		newHTTPClient("core-agent", "https://"+net.JoinHostPort(address, strconv.Itoa(port))+"/agent/eudm-capture", auth.GetAuthToken(), client),
		newHTTPClient("process-agent", "https://"+processAddress+"/eudm-capture", auth.GetAuthToken(), client),
	}
	closeClients := func() { transport.CloseIdleConnections() }
	if includeProbe {
		socket := cfg.GetString("system_probe_config.sysprobe_socket")
		if probeconfig.ValidateSocketAddress(socket) != nil {
			closeClients()
			return nil, nil, errors.New("invalid local system-probe API address")
		}
		// Reuse the Agent socket/named-pipe client, including Windows transport.
		probe := *probeclient.Get(socket)
		probe.Transport = probe.Transport.(*http.Transport).Clone()
		clients = append(clients, newHTTPClient("system-probe", probeclient.URL("eudm-capture"), auth.GetAuthToken(), &probe))
		closeClients = func() { transport.CloseIdleConnections(); probe.CloseIdleConnections() }
	}
	return clients, closeClients, nil
}

// RunInstalled arms existing compatible services. It creates no credentials,
// collectors, listeners, or services, and never changes installed configuration.
func RunInstalled(ctx context.Context, directory, cfgpath string) error {
	platform := runtime.GOOS
	if platform == "darwin" {
		platform = "macos"
	}
	if platform != "macos" && platform != "windows" {
		return errors.New("live capture is supported only on macOS and Windows")
	}
	if len(version.FullCommit) != 40 || strings.Trim(version.FullCommit, "0123456789abcdef") != "" {
		return errors.New("capture requires a revision-stamped build; use dda inv eudm-simulator.build")
	}
	// Configuration and IPC helpers can log paths and artifact fingerprints.
	// Capture emits only its own fixed control-plane errors.
	log.SetupLogger(log.Disabled(), "error")
	cfg, err := installedConfig(cfgpath, platform == "windows")
	if err != nil {
		return err
	}
	var auth ipc.Component
	app := fx.New(fx.NopLogger, fx.Provide(func() config.Component { return cfg }),
		fx.Provide(func() logdef.Component { return log.NewWrapper(2) }),
		ipcfx.ModuleReadOnly(), fx.Populate(&auth))
	if app.Err() != nil {
		return errors.New("cannot read existing Agent IPC authentication artifacts; check access and running services")
	}
	if app.Start(ctx) != nil {
		return errors.New("cannot initialize read-only Agent IPC authentication")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = app.Stop(cleanup)
	}()
	clients, closeClients, err := installedClients(cfg, auth, platform == "windows")
	if err != nil {
		return err
	}
	defer closeClients()
	evidence := NewEvidence(directory, platform, runtime.GOARCH, bundle.BuildIdentity{Version: version.AgentVersion, Commit: version.FullCommit})
	defer evidence.Close()
	return Run(ctx, platform, clients, evidence)
}
