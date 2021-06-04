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

	model "github.com/DataDog/agent-payload/process"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/config"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	ddgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"google.golang.org/grpc"
)

const (
	// defaultProxyPort is the default port used for proxies.
	// This mirrors the configuration for the infrastructure agent.
	defaultProxyPort = 3128

	defaultGRPCConnectionTimeout = 60 * time.Second
)

// Name for check performed by process-agent or system-probe
const (
	ProcessCheckName     = "process"
	RTProcessCheckName   = "rtprocess"
	ContainerCheckName   = "container"
	RTContainerCheckName = "rtcontainer"
	ConnectionsCheckName = "connections"
	PodCheckName         = "pod"

	NetworkCheckName        = "Network"
	OOMKillCheckName        = "OOM Kill"
	TCPQueueLengthCheckName = "TCP queue length"
	ProcessModuleCheckName  = "Process Module"
)

var (
	processChecks   = []string{ProcessCheckName, RTProcessCheckName}
	containerChecks = []string{ContainerCheckName, RTContainerCheckName}

	moduleCheckMap = map[sysconfig.ModuleName][]string{
		sysconfig.NetworkTracerModule:        {ConnectionsCheckName, NetworkCheckName},
		sysconfig.OOMKillProbeModule:         {OOMKillCheckName},
		sysconfig.TCPQueueLengthTracerModule: {TCPQueueLengthCheckName},
		sysconfig.ProcessModule:              {ProcessModuleCheckName},
	}
)

type proxyFunc func(*http.Request) (*url.URL, error)

type cmdFunc = func(name string, arg ...string) *exec.Cmd

// WindowsConfig stores all windows-specific configuration for the process-agent and system-probe.
type WindowsConfig struct {
	// Number of checks runs between refreshes of command-line arguments
	ArgsRefreshInterval int
	// Controls getting process arguments immediately when a new process is discovered
	AddNewArgs bool
}

// AgentConfig is the global config for the process-agent. This information
// is sourced from config files and the environment variables.
type AgentConfig struct {
	Enabled                   bool
	HostName                  string
	APIEndpoints              []apicfg.Endpoint
	LogFile                   string
	LogLevel                  string
	LogToConsole              bool
	QueueSize                 int // The number of items allowed in each delivery queue.
	ProcessQueueBytes         int // The total number of bytes that can be enqueued for delivery to the process intake endpoint
	Blacklist                 []*regexp.Regexp
	Scrubber                  *DataScrubber
	MaxPerMessage             int
	MaxCtrProcessesPerMessage int // The maximum number of processes that belong to a container for a given message
	MaxConnsPerMessage        int
	AllowRealTime             bool
	Transport                 *http.Transport `json:"-"`
	DDAgentBin                string
	StatsdHost                string
	StatsdPort                int
	ProcessExpVarPort         int
	ProfilingEnabled          bool
	ProfilingSite             string
	ProfilingURL              string
	ProfilingAPIKey           string
	ProfilingEnvironment      string
	ProfilingPeriod           time.Duration
	ProfilingCPUDuration      time.Duration
	// host type of the agent, used to populate container payload with additional host information
	ContainerHostType model.ContainerHostType

	// System probe collection configuration
	EnableSystemProbe  bool
	SystemProbeAddress string

	// Orchestrator config
	Orchestrator *oconfig.OrchestratorConfig

	// Check config
	EnabledChecks  []string
	CheckIntervals map[string]time.Duration

	// Internal store of a proxy used for generating the Transport
	proxy proxyFunc

	// Windows-specific config
	Windows WindowsConfig

	grpcConnectionTimeout time.Duration
}

// CheckIsEnabled returns a bool indicating if the given check name is enabled.
func (a AgentConfig) CheckIsEnabled(checkName string) bool {
	return util.StringInSlice(a.EnabledChecks, checkName)
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

const (
	defaultProcessEndpoint         = "https://process.datadoghq.com"
	maxMessageBatch                = 100
	defaultMaxCtrProcsMessageBatch = 10000
	maxCtrProcsMessageBatch        = 30000
)

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
func NewDefaultAgentConfig(canAccessContainers bool) *AgentConfig {
	processEndpoint, err := url.Parse(defaultProcessEndpoint)
	if err != nil {
		// This is a hardcoded URL so parsing it should not fail
		panic(err)
	}

	var enabledChecks []string
	if canAccessContainers {
		enabledChecks = containerChecks
	}

	ac := &AgentConfig{
		Enabled:      canAccessContainers, // We'll always run inside of a container.
		APIEndpoints: []apicfg.Endpoint{{Endpoint: processEndpoint}},
		LogFile:      defaultLogFilePath,
		LogLevel:     "info",
		LogToConsole: false,

		// Allow buffering up to 75 megabytes of payload data in total
		ProcessQueueBytes: 60 * 1000 * 1000,
		// This can be fairly high as the input should get throttled by queue bytes first.
		// Assuming we generate ~8 checks/minute (for process/network), this should allow buffering of ~30 minutes of data assuming it fits within the queue bytes memory budget
		QueueSize: 256,

		MaxPerMessage:             maxMessageBatch,
		MaxCtrProcessesPerMessage: defaultMaxCtrProcsMessageBatch,
		MaxConnsPerMessage:        600,
		AllowRealTime:             true,
		HostName:                  "",
		Transport:                 NewDefaultTransport(),
		ProcessExpVarPort:         6062,
		ContainerHostType:         model.ContainerHostType_notSpecified,

		// Statsd for internal instrumentation
		StatsdHost: "127.0.0.1",
		StatsdPort: 8125,

		// System probe collection configuration
		EnableSystemProbe:  false,
		SystemProbeAddress: defaultSystemProbeAddress,

		// Orchestrator config
		Orchestrator: oconfig.NewDefaultOrchestratorConfig(),

		// Check config
		EnabledChecks: enabledChecks,
		CheckIntervals: map[string]time.Duration{
			ProcessCheckName:     10 * time.Second,
			RTProcessCheckName:   2 * time.Second,
			ContainerCheckName:   10 * time.Second,
			RTContainerCheckName: 2 * time.Second,
			ConnectionsCheckName: 30 * time.Second,
			PodCheckName:         10 * time.Second,
		},

		// DataScrubber to hide command line sensitive words
		Scrubber:  NewDefaultDataScrubber(),
		Blacklist: make([]*regexp.Regexp, 0),

		// Windows process config
		Windows: WindowsConfig{
			ArgsRefreshInterval: 15, // with default 20s check interval we refresh every 5m
			AddNewArgs:          true,
		},

		grpcConnectionTimeout: defaultGRPCConnectionTimeout,
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

func loadConfigIfExists(path string) error {
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
func NewAgentConfig(loggerName config.LoggerName, yamlPath, netYamlPath string) (*AgentConfig, error) {
	var err error

	// Note: This only considers container sources that are already setup. It's possible that container sources may
	//       need a few minutes to be ready on newly provisioned hosts.
	_, err = util.GetContainers()
	canAccessContainers := err == nil

	cfg := NewDefaultAgentConfig(canAccessContainers)
	// For Agent 6 we will have a YAML config file to use.
	if err := loadConfigIfExists(yamlPath); err != nil {
		return nil, err
	}

	if err := cfg.LoadProcessYamlConfig(yamlPath); err != nil {
		return nil, err
	}

	if err := cfg.Orchestrator.Load(); err != nil {
		return nil, err
	}

	// (Re)configure the logging from our configuration
	if err := setupLogger(loggerName, cfg.LogFile, cfg); err != nil {
		log.Errorf("failed to setup configured logger: %s", err)
		return nil, err
	}

	// For system probe, there is an additional config file that is shared with the system-probe
	syscfg, err := sysconfig.Merge(netYamlPath)
	if err != nil {
		return nil, err
	}
	if syscfg.Enabled {
		cfg.EnableSystemProbe = true
		cfg.MaxConnsPerMessage = syscfg.MaxConnsPerMessage
		cfg.SystemProbeAddress = syscfg.SocketAddress

		// enable corresponding checks to system-probe modules
		for mod := range syscfg.EnabledModules {
			if checks, ok := moduleCheckMap[mod]; ok {
				cfg.EnabledChecks = append(cfg.EnabledChecks, checks...)
			}
		}

		if !cfg.Enabled {
			log.Info("enabling process-agent for connections check as the system-probe is enabled")
			cfg.Enabled = true
		}
	}

	// TODO: Once proxies have been moved to common config util, remove this
	if cfg.proxy, err = proxyFromEnv(cfg.proxy); err != nil {
		log.Errorf("error parsing environment proxy settings, not using a proxy: %s", err)
		cfg.proxy = nil
	}

	// Python-style log level has WARNING vs WARN
	if strings.ToLower(cfg.LogLevel) == "warning" {
		cfg.LogLevel = "warn"
	}

	if err := validate.ValidHostname(cfg.HostName); err != nil {
		// lookup hostname if there is no config override or if the override is invalid
		if hostname, err := getHostname(cfg.DDAgentBin, cfg.grpcConnectionTimeout); err == nil {
			cfg.HostName = hostname
		} else {
			log.Errorf("Cannot get hostname: %v", err)
		}
	}

	cfg.ContainerHostType = getContainerHostType()

	if cfg.proxy != nil {
		cfg.Transport.Proxy = cfg.proxy
	}

	// sanity check. This element is used with the modulo operator (%), so it can't be zero.
	// if it is, log the error, and assume the config was attempting to disable
	if cfg.Windows.ArgsRefreshInterval == 0 {
		log.Warnf("invalid configuration: windows_collect_skip_new_args was set to 0.  Disabling argument collection")
		cfg.Windows.ArgsRefreshInterval = -1
	}

	// activate the pod collection if enabled and we have the cluster name set
	if cfg.Orchestrator.OrchestrationCollectionEnabled {
		if cfg.Orchestrator.KubeClusterName != "" {
			cfg.EnabledChecks = append(cfg.EnabledChecks, PodCheckName)
		} else {
			log.Warnf("Failed to auto-detect a Kubernetes cluster name. Pod collection will not start. To fix this, set it manually via the cluster_name config option")
		}
	}

	return cfg, nil
}

// getContainerHostType uses the fargate library to detect container environment and returns the protobuf version of it
func getContainerHostType() model.ContainerHostType {
	switch fargate.GetOrchestrator() {
	case fargate.ECS:
		return model.ContainerHostType_fargateECS
	case fargate.EKS:
		return model.ContainerHostType_fargateEKS
	}
	return model.ContainerHostType_notSpecified
}

func loadEnvVariables() {
	// The following environment variables will be loaded in the order listed, meaning variables
	// further down the list may override prior variables.
	for _, variable := range []struct{ env, cfg string }{
		{"DD_PROCESS_AGENT_CONTAINER_SOURCE", "process_config.container_source"},
		{"DD_SCRUB_ARGS", "process_config.scrub_args"},
		{"DD_STRIP_PROCESS_ARGS", "process_config.strip_proc_arguments"},
		{"DD_PROCESS_AGENT_URL", "process_config.process_dd_url"},
		{"DD_PROCESS_AGENT_INTERNAL_PROFILING_ENABLED", "process_config.internal_profiling.enabled"},
		{"DD_PROCESS_AGENT_REMOTE_TAGGER", "process_config.remote_tagger"},
		{"DD_PROCESS_AGENT_MAX_PER_MESSAGE", "process_config.max_per_message"},
		{"DD_PROCESS_AGENT_MAX_CTR_PROCS_PER_MESSAGE", "process_config.max_ctr_procs_per_message"},
		{"DD_ORCHESTRATOR_URL", "orchestrator_explorer.orchestrator_dd_url"},
		{"DD_HOSTNAME", "hostname"},
		{"DD_DOGSTATSD_PORT", "dogstatsd_port"},
		{"DD_BIND_HOST", "bind_host"},
		{"HTTPS_PROXY", "proxy.https"},
		{"DD_PROXY_HTTPS", "proxy.https"},

		{"DD_LOGS_STDOUT", "log_to_console"},
		{"LOG_TO_CONSOLE", "log_to_console"},
		{"DD_LOG_TO_CONSOLE", "log_to_console"},
		{"LOG_LEVEL", "log_level"}, // Support LOG_LEVEL and DD_LOG_LEVEL but prefer DD_LOG_LEVEL
		{"DD_LOG_LEVEL", "log_level"},
	} {
		if v, ok := os.LookupEnv(variable.env); ok {
			config.Datadog.Set(variable.cfg, v)
		}
	}

	// Support API_KEY and DD_API_KEY but prefer DD_API_KEY.
	apiKey, envKey := os.Getenv("DD_API_KEY"), "DD_API_KEY"
	if apiKey == "" {
		apiKey, envKey = os.Getenv("API_KEY"), "API_KEY"
	}

	if apiKey != "" { // We don't want to overwrite the API KEY provided as an environment variable
		log.Infof("overriding API key from env %s value", envKey)
		config.Datadog.Set("api_key", config.SanitizeAPIKey(strings.Split(apiKey, ",")[0]))
	}

	if v := os.Getenv("DD_CUSTOM_SENSITIVE_WORDS"); v != "" {
		config.Datadog.Set("process_config.custom_sensitive_words", strings.Split(v, ","))
	}

	if v := os.Getenv("DD_PROCESS_ADDITIONAL_ENDPOINTS"); v != "" {
		endpoints := make(map[string][]string)
		if err := json.Unmarshal([]byte(v), &endpoints); err != nil {
			log.Errorf(`Could not parse DD_PROCESS_ADDITIONAL_ENDPOINTS: %v. It must be of the form '{"https://process.agent.datadoghq.com": ["apikey1", ...], ...}'.`, err)
		} else {
			config.Datadog.Set("process_config.additional_endpoints", endpoints)
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

func isAffirmative(value string) (bool, error) {
	if value == "" {
		return false, fmt.Errorf("value is empty")
	}
	v := strings.ToLower(value)
	return v == "true" || v == "yes" || v == "1", nil
}

// getHostname attempts to resolve the hostname in the following order: the main datadog agent via grpc, the main agent
// via cli and lastly falling back to os.Hostname() if it is unavailable
func getHostname(ddAgentBin string, grpcConnectionTimeout time.Duration) (string, error) {
	// Fargate is handled as an exceptional case (there is no concept of a host, so we use the ARN in-place).
	if fargate.IsFargateInstance() {
		hostname, err := fargate.GetFargateHost()
		if err == nil {
			return hostname, nil
		}
		log.Errorf("failed to get Fargate host: %v", err)
	}

	// Get the hostname via gRPC from the main agent if a hostname has not been set either from config/fargate
	hostname, err := getHostnameFromGRPC(ddgrpc.GetDDAgentClient, grpcConnectionTimeout)
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
func getHostnameFromGRPC(grpcClientFn func(ctx context.Context, opts ...grpc.DialOption) (pb.AgentClient, error), grpcConnectionTimeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), grpcConnectionTimeout)
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
		cfg.LogLevel,
		logFile,
		config.GetSyslogURI(),
		config.Datadog.GetBool("syslog_rfc"),
		config.Datadog.GetBool("log_to_console"),
		config.Datadog.GetBool("log_format_json"),
	)
}
