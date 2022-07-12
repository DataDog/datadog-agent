// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	ddgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"google.golang.org/grpc"
)

// defaultProxyPort is the default port used for proxies.
// This mirrors the configuration for the infrastructure agent.
const defaultProxyPort = 3128

// Name for check performed by process-agent or system-probe
const (
	ProcessCheckName       = "process"
	RTProcessCheckName     = "rtprocess"
	ContainerCheckName     = "container"
	RTContainerCheckName   = "rtcontainer"
	ConnectionsCheckName   = "connections"
	PodCheckName           = "pod"
	DiscoveryCheckName     = "process_discovery"
	ProcessEventsCheckName = "process_events"

	ProcessCheckDefaultInterval          = 10 * time.Second
	RTProcessCheckDefaultInterval        = 2 * time.Second
	ContainerCheckDefaultInterval        = 10 * time.Second
	RTContainerCheckDefaultInterval      = 2 * time.Second
	ConnectionsCheckDefaultInterval      = 30 * time.Second
	PodCheckDefaultInterval              = 10 * time.Second
	ProcessDiscoveryCheckDefaultInterval = 4 * time.Hour
)

type proxyFunc func(*http.Request) (*url.URL, error)

type cmdFunc = func(name string, arg ...string) *exec.Cmd

// AgentConfig is the global config for the process-agent. This information
// is sourced from config files and the environment variables.
// AgentConfig is shared across process-agent checks and should only contain shared objects and
// settings that cannot be read directly from the global Config object.
// For any other setting, use `pkg/config`.
type AgentConfig struct {
	HostName           string
	Blacklist          []*regexp.Regexp
	Scrubber           *DataScrubber
	MaxConnsPerMessage int
	Transport          *http.Transport `json:"-"`

	// host type of the agent, used to populate container payload with additional host information
	ContainerHostType model.ContainerHostType

	// System probe collection configuration
	EnableSystemProbe  bool
	SystemProbeAddress string

	// Orchestrator config
	Orchestrator *oconfig.OrchestratorConfig

	// Check config
	CheckIntervals map[string]time.Duration

	// Internal store of a proxy used for generating the Transport
	proxy proxyFunc
}

// CheckInterval returns the interval for the given check name, defaulting to 10s if not found.
func (a AgentConfig) CheckInterval(checkName string) time.Duration {
	d, ok := a.CheckIntervals[checkName]
	if !ok {
		log.Errorf("missing check interval for '%s', you must set a default", checkName)
		d = 10 * time.Second
	}
	return d
}

// NewDefaultTransport provides a http transport configuration with sane default timeouts
func NewDefaultTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:    5,
		IdleConnTimeout: 90 * time.Second,
		Dial: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 10 * time.Second,
		}).Dial,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// NewDefaultAgentConfig returns an AgentConfig with defaults initialized
func NewDefaultAgentConfig() *AgentConfig {
	ac := &AgentConfig{
		MaxConnsPerMessage: 600,
		HostName:           "",
		Transport:          NewDefaultTransport(),

		ContainerHostType: model.ContainerHostType_notSpecified,

		// System probe collection configuration
		EnableSystemProbe:  false,
		SystemProbeAddress: defaultSystemProbeAddress,

		// Orchestrator config
		Orchestrator: oconfig.NewDefaultOrchestratorConfig(),

		// Check config
		CheckIntervals: map[string]time.Duration{
			ProcessCheckName:       ProcessCheckDefaultInterval,
			RTProcessCheckName:     RTProcessCheckDefaultInterval,
			ContainerCheckName:     ContainerCheckDefaultInterval,
			RTContainerCheckName:   RTContainerCheckDefaultInterval,
			ConnectionsCheckName:   ConnectionsCheckDefaultInterval,
			PodCheckName:           PodCheckDefaultInterval,
			DiscoveryCheckName:     ProcessDiscoveryCheckDefaultInterval,
			ProcessEventsCheckName: config.DefaultProcessEventsCheckInterval,
		},

		// DataScrubber to hide command line sensitive words
		Scrubber:  NewDefaultDataScrubber(),
		Blacklist: make([]*regexp.Regexp, 0),
	}

	// Set default values for proc/sys paths if unset.
	// Don't set this is /host is not mounted to use context within container.
	// Generally only applicable for container-only cases like Fargate.
	if config.IsContainerized() && util.PathExists("/host") {
		if v := os.Getenv("HOST_PROC"); v == "" {
			os.Setenv("HOST_PROC", "/host/proc")
		}
		if v := os.Getenv("HOST_SYS"); v == "" {
			os.Setenv("HOST_SYS", "/host/sys")
		}
	}

	return ac
}

// LoadConfigIfExists takes a path to either a directory containing datadog.yaml or a direct path to a datadog.yaml file
// and loads it into ddconfig.Datadog. It does this silently, and does not produce any logs.
func LoadConfigIfExists(path string) error {
	if path != "" {
		if util.PathExists(path) {
			config.Datadog.AddConfigPath(path)
			if strings.HasSuffix(path, ".yaml") { // If they set a config file directly, let's try to honor that
				config.Datadog.SetConfigFile(path)
			}

			if _, err := config.LoadWithoutSecret(); err != nil {
				return err
			}
		} else {
			log.Infof("no config exists at %s, ignoring...", path)
		}
	}
	return nil
}

// NewAgentConfig returns an AgentConfig using a configuration file. It can be nil
// if there is no file available. In this case we'll configure only via environment.
func NewAgentConfig(loggerName config.LoggerName, yamlPath string, syscfg *sysconfig.Config) (*AgentConfig, error) {
	var err error

	cfg := NewDefaultAgentConfig()
	if err := cfg.LoadAgentConfig(yamlPath); err != nil {
		return nil, err
	}

	if err := cfg.Orchestrator.Load(); err != nil {
		return nil, err
	}

	// (Re)configure the logging from our configuration
	logFile := config.Datadog.GetString("process_config.log_file")
	if err := setupLogger(loggerName, logFile, cfg); err != nil {
		log.Errorf("failed to setup configured logger: %s", err)
		return nil, err
	}

	if syscfg.Enabled {
		cfg.EnableSystemProbe = true
		cfg.MaxConnsPerMessage = syscfg.MaxConnsPerMessage
		cfg.SystemProbeAddress = syscfg.SocketAddress
	}

	// TODO: Once proxies have been moved to common config util, remove this
	if cfg.proxy, err = proxyFromEnv(cfg.proxy); err != nil {
		log.Errorf("error parsing environment proxy settings, not using a proxy: %s", err)
		cfg.proxy = nil
	}

	if err := validate.ValidHostname(cfg.HostName); err != nil {
		// lookup hostname if there is no config override or if the override is invalid
		agentBin := config.Datadog.GetString("process_config.dd_agent_bin")
		connectionTimeout := config.Datadog.GetDuration("process_config.grpc_connection_timeout_secs") * time.Second
		if hostname, err := getHostname(context.TODO(), agentBin, connectionTimeout); err == nil {
			cfg.HostName = hostname
		} else {
			log.Errorf("Cannot get hostname: %v", err)
		}
	}

	cfg.ContainerHostType = getContainerHostType()

	if cfg.proxy != nil {
		cfg.Transport.Proxy = cfg.proxy
	}

	return cfg, nil
}

// InitRuntimeSettings registers settings to be added to the runtime config.
func InitRuntimeSettings() {
	// NOTE: Any settings you want to register should simply be added here
	processRuntimeSettings := []settings.RuntimeSetting{
		settings.LogLevelRuntimeSetting{},
	}

	// Before we begin listening, register runtime settings
	for _, setting := range processRuntimeSettings {
		err := settings.RegisterRuntimeSetting(setting)
		if err != nil {
			_ = log.Warnf("cannot initialize the runtime setting %s: %v", setting.Name(), err)
		}
	}
}

// getContainerHostType uses the fargate library to detect container environment and returns the protobuf version of it
func getContainerHostType() model.ContainerHostType {
	switch fargate.GetOrchestrator(context.TODO()) {
	case fargate.ECS:
		return model.ContainerHostType_fargateECS
	case fargate.EKS:
		return model.ContainerHostType_fargateEKS
	}
	return model.ContainerHostType_notSpecified
}

// loadEnvVariables reads env variables specific to process-agent and overrides the corresponding settings
// in the global Config object.
// This function is used to handle historic process-agent env vars. New settings should be
// handled directly in the /pkg/config/process.go file
func loadEnvVariables() {
	// The following environment variables will be loaded in the order listed, meaning variables
	// further down the list may override prior variables.
	for _, variable := range []struct{ env, cfg string }{
		{"DD_ORCHESTRATOR_URL", "orchestrator_explorer.orchestrator_dd_url"},
		{"HTTPS_PROXY", "proxy.https"},
	} {
		if v, ok := os.LookupEnv(variable.env); ok {
			config.Datadog.Set(variable.cfg, v)
		}
	}

	if v := os.Getenv("DD_ORCHESTRATOR_ADDITIONAL_ENDPOINTS"); v != "" {
		endpoints := make(map[string][]string)
		if err := json.Unmarshal([]byte(v), &endpoints); err != nil {
			log.Errorf(`Could not parse DD_ORCHESTRATOR_ADDITIONAL_ENDPOINTS: %v. It must be of the form '{"https://process.agent.datadoghq.com": ["apikey1", ...], ...}'.`, err)
		} else {
			config.Datadog.Set("orchestrator_explorer.orchestrator_additional_endpoints", endpoints)
		}
	}
}

// IsBlacklisted returns a boolean indicating if the given command is blacklisted by our config.
func IsBlacklisted(cmdline []string, blacklist []*regexp.Regexp) bool {
	cmd := strings.Join(cmdline, " ")
	for _, b := range blacklist {
		if b.MatchString(cmd) {
			return true
		}
	}
	return false
}

// getHostname attempts to resolve the hostname in the following order: the main datadog agent via grpc, the main agent
// via cli and lastly falling back to os.Hostname() if it is unavailable
func getHostname(ctx context.Context, ddAgentBin string, grpcConnectionTimeout time.Duration) (string, error) {
	// Fargate is handled as an exceptional case (there is no concept of a host, so we use the ARN in-place).
	if fargate.IsFargateInstance(ctx) {
		hostname, err := fargate.GetFargateHost(ctx)
		if err == nil {
			return hostname, nil
		}
		log.Errorf("failed to get Fargate host: %v", err)
	}

	// Get the hostname via gRPC from the main agent if a hostname has not been set either from config/fargate
	hostname, err := getHostnameFromGRPC(ctx, ddgrpc.GetDDAgentClient, grpcConnectionTimeout)
	if err == nil {
		return hostname, nil
	}
	log.Errorf("failed to get hostname from grpc: %v", err)

	// If the hostname is not set then we fallback to use the agent binary
	hostname, err = getHostnameFromCmd(ddAgentBin, exec.Command)
	if err == nil {
		return hostname, nil
	}
	log.Errorf("failed to get hostname from cmd: %v", err)

	return os.Hostname()
}

// getHostnameCmd shells out to obtain the hostname used by the infra agent
func getHostnameFromCmd(ddAgentBin string, cmdFn cmdFunc) (string, error) {
	cmd := cmdFn(ddAgentBin, "hostname")

	// Copying all environment variables to child process
	// Windows: Required, so the child process can load DLLs, etc.
	// Linux:   Optional, but will make use of DD_HOSTNAME and DOCKER_DD_AGENT if they exist
	cmd.Env = append(cmd.Env, os.Environ()...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	hostname := strings.TrimSpace(stdout.String())
	if hostname == "" {
		return "", fmt.Errorf("error retrieving dd-agent hostname %s", stderr.String())
	}

	return hostname, nil
}

// getHostnameFromGRPC retrieves the hostname from the main datadog agent via GRPC
func getHostnameFromGRPC(ctx context.Context, grpcClientFn func(ctx context.Context, opts ...grpc.DialOption) (pb.AgentClient, error), grpcConnectionTimeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, grpcConnectionTimeout)
	defer cancel()

	ddAgentClient, err := grpcClientFn(ctx)
	if err != nil {
		return "", fmt.Errorf("cannot connect to datadog agent via grpc: %w", err)
	}
	reply, err := ddAgentClient.GetHostname(ctx, &pb.HostnameRequest{})
	if err != nil {
		return "", fmt.Errorf("cannot get hostname from datadog agent via grpc: %w", err)
	}

	log.Debugf("retrieved hostname:%s from datadog agent via grpc", reply.Hostname)
	return reply.Hostname, nil
}

// proxyFromEnv parses out the proxy configuration from the ENV variables in a
// similar way to getProxySettings and, if enough values are available, returns
// a new proxy URL value. If the environment is not set for this then the
// `defaultVal` is returned.
func proxyFromEnv(defaultVal proxyFunc) (proxyFunc, error) {
	var host string
	scheme := "http"
	if v := os.Getenv("PROXY_HOST"); v != "" {
		// accept either http://myproxy.com or myproxy.com
		if i := strings.Index(v, "://"); i != -1 {
			// when available, parse the scheme from the url
			scheme = v[0:i]
			host = v[i+3:]
		} else {
			host = v
		}
	}

	if host == "" {
		return defaultVal, nil
	}

	port := defaultProxyPort
	if v := os.Getenv("PROXY_PORT"); v != "" {
		port, _ = strconv.Atoi(v)
	}
	var user, password string
	if v := os.Getenv("PROXY_USER"); v != "" {
		user = v
	}
	if v := os.Getenv("PROXY_PASSWORD"); v != "" {
		password = v
	}

	return constructProxy(host, scheme, port, user, password)
}

// constructProxy constructs a *url.Url for a proxy given the parts of a
// Note that we assume we have at least a non-empty host for this call but
// all other values can be their defaults (empty string or 0).
func constructProxy(host, scheme string, port int, user, password string) (proxyFunc, error) {
	var userpass *url.Userinfo
	if user != "" {
		if password != "" {
			userpass = url.UserPassword(user, password)
		} else {
			userpass = url.User(user)
		}
	}

	var path string
	if userpass != nil {
		path = fmt.Sprintf("%s@%s:%v", userpass.String(), host, port)
	} else {
		path = fmt.Sprintf("%s:%v", host, port)
	}
	if scheme != "" {
		path = fmt.Sprintf("%s://%s", scheme, path)
	}

	u, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	return http.ProxyURL(u), nil
}

func setupLogger(loggerName config.LoggerName, logFile string, cfg *AgentConfig) error {
	return config.SetupLogger(
		loggerName,
		config.Datadog.GetString("log_level"),
		logFile,
		config.GetSyslogURI(),
		config.Datadog.GetBool("syslog_rfc"),
		config.Datadog.GetBool("log_to_console"),
		config.Datadog.GetBool("log_format_json"),
	)
}
