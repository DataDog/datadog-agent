package config

import (
	"bytes"
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
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// defaultProxyPort is the default port used for proxies.
	// This mirrors the configuration for the infrastructure agent.
	defaultProxyPort = 3128

	// defaultSystemProbeBPFDir is the default path for eBPF programs
	defaultSystemProbeBPFDir = "/opt/datadog-agent/embedded/share/system-probe/ebpf"

	processChecks   = []string{"process", "rtprocess"}
	containerChecks = []string{"container", "rtcontainer"}
)

type proxyFunc func(*http.Request) (*url.URL, error)

// WindowsConfig stores all windows-specific configuration for the process-agent and system-probe.
type WindowsConfig struct {
	// Number of checks runs between refreshes of command-line arguments
	ArgsRefreshInterval int
	// Controls getting process arguments immediately when a new process is discovered
	AddNewArgs bool

	//System Probe Configuration

	// EnableMonotonicCount determines if we will calculate send/recv bytes of connections with headers and retransmits
	EnableMonotonicCount bool

	// DriverBufferSize (bytes) determines the size of the buffer we pass to the driver when reading flows
	DriverBufferSize int
}

// AgentConfig is the global config for the process-agent. This information
// is sourced from config files and the environment variables.
type AgentConfig struct {
	Enabled               bool
	HostName              string
	APIEndpoints          []api.Endpoint
	OrchestratorEndpoints []api.Endpoint
	LogFile               string
	LogLevel              string
	LogToConsole          bool
	QueueSize             int // The number of items allowed in each delivery queue.
	ProcessQueueBytes     int // The total number of bytes that can be enqueued for delivery to the process intake endpoint
	PodQueueBytes         int // The total number of bytes that can be enqueued for delivery to the orchestrator endpoint
	Blacklist             []*regexp.Regexp
	Scrubber              *DataScrubber
	MaxPerMessage         int
	MaxConnsPerMessage    int
	AllowRealTime         bool
	Transport             *http.Transport `json:"-"`
	DDAgentBin            string
	StatsdHost            string
	StatsdPort            int
	ProcessExpVarPort     int
	// host type of the agent, used to populate container payload with additional host information
	ContainerHostType model.ContainerHostType

	// System probe collection configuration
	EnableSystemProbe              bool
	DisableTCPTracing              bool
	DisableUDPTracing              bool
	DisableIPv6Tracing             bool
	DisableDNSInspection           bool
	CollectLocalDNS                bool
	SystemProbeAddress             string
	SystemProbeLogFile             string
	SystemProbeBPFDir              string
	MaxTrackedConnections          uint
	SysProbeBPFDebug               bool
	ExcludedBPFLinuxVersions       []string
	ExcludedSourceConnections      map[string][]string
	ExcludedDestinationConnections map[string][]string
	EnableConntrack                bool
	ConntrackMaxStateSize          int
	ConntrackRateLimit             int
	SystemProbeDebugPort           int
	ClosedChannelSize              int
	MaxClosedConnectionsBuffered   int
	MaxConnectionsStateBuffered    int
	OffsetGuessThreshold           uint64
	EnableTracepoints              bool

	// DNS stats configuration
	CollectDNSStats bool
	DNSTimeout      time.Duration

	// Orchestrator collection configuration
	OrchestrationCollectionEnabled bool
	KubeClusterName                string
	IsScrubbingEnabled             bool

	// Check config
	EnabledChecks  []string
	CheckIntervals map[string]time.Duration

	// Internal store of a proxy used for generating the Transport
	proxy proxyFunc

	// Windows-specific config
	Windows WindowsConfig
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
	defaultProcessEndpoint       = "https://process.datadoghq.com"
	defaultOrchestratorEndpoint  = "https://orchestrator.datadoghq.com"
	maxMessageBatch              = 100
	maxConnsMessageBatch         = 1000
	defaultMaxTrackedConnections = 65536
	maxOffsetThreshold           = 3000
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
	orchestratorEndpoint, err := url.Parse(defaultOrchestratorEndpoint)
	if err != nil {
		// This is a hardcoded URL so parsing it should not fail
		panic(err)
	}

	var enabledChecks []string
	if canAccessContainers {
		enabledChecks = containerChecks
	}

	ac := &AgentConfig{
		Enabled:               canAccessContainers, // We'll always run inside of a container.
		APIEndpoints:          []api.Endpoint{{Endpoint: processEndpoint}},
		OrchestratorEndpoints: []api.Endpoint{{Endpoint: orchestratorEndpoint}},
		LogFile:               defaultLogFilePath,
		LogLevel:              "info",
		LogToConsole:          false,

		// Allow buffering up to 75 megabytes of payload data in total
		ProcessQueueBytes: 60 * 1000 * 1000,
		PodQueueBytes:     15 * 1000 * 1000,
		// This can be fairly high as the input should get throttled by queue bytes first.
		// Assuming we generate ~8 checks/minute (for process/network), this should allow buffering of ~30 minutes of data assuming it fits within the queue bytes memory budget
		QueueSize: 256,

		MaxPerMessage:      100,
		MaxConnsPerMessage: 600,
		AllowRealTime:      true,
		HostName:           "",
		Transport:          NewDefaultTransport(),
		ProcessExpVarPort:  6062,
		ContainerHostType:  model.ContainerHostType_notSpecified,

		// Statsd for internal instrumentation
		StatsdHost: "127.0.0.1",
		StatsdPort: 8125,

		// System probe collection configuration
		EnableSystemProbe:     false,
		DisableTCPTracing:     false,
		DisableUDPTracing:     false,
		DisableIPv6Tracing:    false,
		DisableDNSInspection:  false,
		SystemProbeAddress:    defaultSystemProbeAddress,
		SystemProbeLogFile:    defaultSystemProbeLogFilePath,
		SystemProbeBPFDir:     defaultSystemProbeBPFDir,
		MaxTrackedConnections: defaultMaxTrackedConnections,
		EnableConntrack:       true,
		ClosedChannelSize:     500,
		ConntrackMaxStateSize: defaultMaxTrackedConnections * 2,
		ConntrackRateLimit:    500,
		OffsetGuessThreshold:  400,
		EnableTracepoints:     false,

		// Check config
		EnabledChecks: enabledChecks,
		CheckIntervals: map[string]time.Duration{
			"process":     10 * time.Second,
			"rtprocess":   2 * time.Second,
			"container":   10 * time.Second,
			"rtcontainer": 2 * time.Second,
			"connections": 30 * time.Second,
			"pod":         10 * time.Second,
		},

		// DataScrubber to hide command line sensitive words
		Scrubber:  NewDefaultDataScrubber(),
		Blacklist: make([]*regexp.Regexp, 0),

		// Windows process config
		Windows: WindowsConfig{
			ArgsRefreshInterval:  15, // with default 20s check interval we refresh every 5m
			AddNewArgs:           true,
			EnableMonotonicCount: false,
			DriverBufferSize:     1024,
		},
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
	return nil
}

func mergeConfigIfExists(path string) error {
	if util.PathExists(path) {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		if err := config.Datadog.MergeConfig(file); err != nil {
			return err
		}
	} else {
		log.Infof("no config exists at %s, ignoring...", path)
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

	// (Re)configure the logging from our configuration
	if err := setupLogger(loggerName, cfg.LogFile, cfg); err != nil {
		log.Errorf("failed to setup configured logger: %s", err)
		return nil, err
	}

	// For system probe, there is an additional config file that is shared with the system-probe
	mergeConfigIfExists(netYamlPath) //nolint:errcheck
	if err = cfg.loadSysProbeYamlConfig(netYamlPath); err != nil {
		return nil, err
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

	if cfg.HostName == "" {
		if fargate.IsFargateInstance() {
			if hostname, err := fargate.GetFargateHost(); err == nil {
				cfg.HostName = hostname
			} else {
				log.Errorf("Cannot get Fargate host: %v", err)
			}
		} else if hostname, err := getHostname(cfg.DDAgentBin); err == nil {
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
	if cfg.OrchestrationCollectionEnabled && cfg.KubeClusterName != "" {
		cfg.EnabledChecks = append(cfg.EnabledChecks, "pod")
	}

	return cfg, nil
}

// NewSystemProbeConfig returns a system-probe specific AgentConfig using a configuration file. It can be nil
// if there is no file available. In this case we'll configure only via environment.
func NewSystemProbeConfig(loggerName config.LoggerName, yamlPath string) (*AgentConfig, error) {
	cfg := NewDefaultAgentConfig(false) // We don't access the container APIs in the system-probe

	// When the system-probe is enabled in a separate container, we need a way to also disable the system-probe
	// packaged in the main agent container (without disabling network collection on the process-agent).
	//
	// If this environment flag is set, it'll sure it will not start
	if ok, _ := isAffirmative(os.Getenv("DD_SYSTEM_PROBE_EXTERNAL")); ok {
		cfg.EnableSystemProbe = false
		return cfg, nil
	}

	loadConfigIfExists(yamlPath) //nolint:errcheck
	if err := cfg.loadSysProbeYamlConfig(yamlPath); err != nil {
		return nil, err
	}

	// (Re)configure the logging from our configuration, with the system probe log file + config options
	if err := setupLogger(loggerName, cfg.SystemProbeLogFile, cfg); err != nil {
		log.Errorf("failed to setup configured logger: %s", err)
		return nil, err
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

	// Load the System Probe environment variables
	loadSysProbeEnvVariables()

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

func loadSysProbeEnvVariables() {
	for _, variable := range []struct{ env, cfg string }{
		{"DD_SYSTEM_PROBE_ENABLED", "system_probe_config.enabled"},
		{"DD_SYSPROBE_SOCKET", "system_probe_config.sysprobe_socket"},
		{"DD_SYSTEM_PROBE_CONNTRACK_IGNORE_ENOBUFS", "system_probe_config.conntrack_ignore_enobufs"},
		{"DD_DISABLE_TCP_TRACING", "system_probe_config.disable_tcp"},
		{"DD_DISABLE_UDP_TRACING", "system_probe_config.disable_udp"},
		{"DD_DISABLE_IPV6_TRACING", "system_probe_config.disable_ipv6"},
		{"DD_DISABLE_DNS_INSPECTION", "system_probe_config.disable_dns_inspection"},
		{"DD_COLLECT_LOCAL_DNS", "system_probe_config.collect_local_dns"},
		{"DD_COLLECT_DNS_STATS", "system_probe_config.collect_dns_stats"},
	} {
		if v, ok := os.LookupEnv(variable.env); ok {
			config.Datadog.Set(variable.cfg, v)

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

// getHostname shells out to obtain the hostname used by the infra agent
// falling back to os.Hostname() if it is unavailable
func getHostname(ddAgentBin string) (string, error) {
	cmd := exec.Command(ddAgentBin, "hostname")

	// Copying all environment variables to child process
	// Windows: Required, so the child process can load DLLs, etc.
	// Linux:   Optional, but will make use of DD_HOSTNAME and DOCKER_DD_AGENT if they exist
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		log.Infof("error retrieving dd-agent hostname, falling back to os.Hostname(): %v", err)
		return os.Hostname()
	}

	hostname := strings.TrimSpace(stdout.String())

	if hostname == "" {
		log.Infof("error retrieving dd-agent hostname, falling back to os.Hostname(): %s", stderr.String())
		return os.Hostname()
	}

	return hostname, err
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
