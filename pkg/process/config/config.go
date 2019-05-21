package config

import (
	"bytes"
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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// defaultProxyPort is the default port used for proxies.
	// This mirrors the configuration for the infrastructure agent.
	defaultProxyPort = 3128

	// defaultNetworkTracerSocketPath is the default unix socket path to be used for connecting to the network tracer
	defaultNetworkTracerSocketPath = "/opt/datadog-agent/run/nettracer.sock"
	// defaultNetworkLogFilePath is the default logging file for the network tracer
	defaultNetworkLogFilePath = "/var/log/datadog/network-tracer.log"

	defaultConntrackShortTermBufferSize = 10000

	processChecks   = []string{"process", "rtprocess"}
	containerChecks = []string{"container", "rtcontainer"}
)

type proxyFunc func(*http.Request) (*url.URL, error)

// WindowsConfig stores all windows-specific configuration for the process-agent.
type WindowsConfig struct {
	// Number of checks runs between refreshes of command-line arguments
	ArgsRefreshInterval int
	// Controls getting process arguments immediately when a new process is discovered
	AddNewArgs bool
}

// APIEndpoint is a single endpoint where process data will be submitted.
type APIEndpoint struct {
	APIKey   string
	Endpoint *url.URL
}

// AgentConfig is the global config for the process-agent. This information
// is sourced from config files and the environment variables.
type AgentConfig struct {
	Enabled            bool
	HostName           string
	APIEndpoints       []APIEndpoint
	LogFile            string
	LogLevel           string
	LogToConsole       bool
	QueueSize          int
	Blacklist          []*regexp.Regexp
	Scrubber           *DataScrubber
	MaxPerMessage      int
	MaxConnsPerMessage int
	AllowRealTime      bool
	Transport          *http.Transport `json:"-"`
	DDAgentBin         string
	StatsdHost         string
	StatsdPort         int
	ProcessExpVarPort  int

	// Network collection configuration
	EnableNetworkTracing         bool
	EnableLocalNetworkTracer     bool // To have the network tracer embedded in the process-agent
	EnableDebugProfiling         bool
	DisableTCPTracing            bool
	DisableUDPTracing            bool
	DisableIPv6Tracing           bool
	CollectLocalDNS              bool
	NetworkTracerSocketPath      string
	NetworkTracerLogFile         string
	MaxTrackedConnections        uint
	NetworkBPFDebug              bool
	ExcludedBPFLinuxVersions     []string
	EnableConntrack              bool
	ConntrackShortTermBufferSize int

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
	defaultEndpoint          = "https://process.datadoghq.com"
	maxMessageBatch          = 100
	maxConnsMessageBatch     = 300
	maxMaxTrackedConnections = 65536
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
func NewDefaultAgentConfig() *AgentConfig {
	u, err := url.Parse(defaultEndpoint)
	if err != nil {
		// This is a hardcoded URL so parsing it should not fail
		panic(err)
	}

	// Note: This only considers container sources that are already setup. It's possible that container sources may
	//       need a few minutes to be ready.
	_, err = util.GetContainers()
	canAccessContainers := err == nil

	ac := &AgentConfig{
		Enabled:            canAccessContainers, // We'll always run inside of a container.
		APIEndpoints:       []APIEndpoint{{Endpoint: u}},
		LogFile:            defaultLogFilePath,
		LogLevel:           "info",
		LogToConsole:       false,
		QueueSize:          20,
		MaxPerMessage:      100,
		MaxConnsPerMessage: 300,
		AllowRealTime:      true,
		HostName:           "",
		Transport:          NewDefaultTransport(),
		ProcessExpVarPort:  6062,

		// Statsd for internal instrumentation
		StatsdHost: "127.0.0.1",
		StatsdPort: 8125,

		// Network collection configuration
		EnableNetworkTracing:         false,
		EnableLocalNetworkTracer:     false,
		DisableTCPTracing:            false,
		DisableUDPTracing:            false,
		DisableIPv6Tracing:           false,
		NetworkTracerSocketPath:      defaultNetworkTracerSocketPath,
		NetworkTracerLogFile:         defaultNetworkLogFilePath,
		MaxTrackedConnections:        maxMaxTrackedConnections,
		EnableConntrack:              true,
		ConntrackShortTermBufferSize: defaultConntrackShortTermBufferSize,

		// Check config
		EnabledChecks: containerChecks,
		CheckIntervals: map[string]time.Duration{
			"process":     10 * time.Second,
			"rtprocess":   2 * time.Second,
			"container":   10 * time.Second,
			"rtcontainer": 2 * time.Second,
			"connections": 30 * time.Second,
		},

		// DataScrubber to hide command line sensitive words
		Scrubber:  NewDefaultDataScrubber(),
		Blacklist: make([]*regexp.Regexp, 0),

		// Windows process config
		Windows: WindowsConfig{
			ArgsRefreshInterval: 15, // with default 20s check interval we refresh every 5m
			AddNewArgs:          true,
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

		if err := config.Load(); err != nil {
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
	cfg := NewDefaultAgentConfig()

	// For Agent 6 we will have a YAML config file to use.
	loadConfigIfExists(yamlPath)
	if err := cfg.loadProcessYamlConfig(yamlPath); err != nil {
		return nil, err
	}

	// (Re)configure the logging from our configuration
	if err := setupLogger(loggerName, cfg.LogFile, cfg); err != nil {
		log.Errorf("failed to setup configured logger: %s", err)
		return nil, err
	}

	// For network tracing, there is an additional config file that is shared with the network-tracer
	loadConfigIfExists(netYamlPath)
	if err = cfg.loadNetworkYamlConfig(netYamlPath); err != nil {
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

	if v := os.Getenv("DD_HOSTNAME"); v != "" {
		log.Info("overriding hostname from env DD_HOSTNAME value")
		cfg.HostName = v
	}

	if cfg.HostName == "" {
		if ecsutil.IsFargateInstance() {
			// Fargate tasks should have no concept of host names, so we're using the task ARN.
			if taskMeta, err := ecsutil.GetTaskMetadata(); err == nil {
				cfg.HostName = fmt.Sprintf("fargate_task:%s", taskMeta.TaskARN)
			} else {
				log.Errorf("Failed to retrieve Fargate task metadata: %s", err)
			}
		} else if hostname, err := getHostname(cfg.DDAgentBin); err == nil {
			cfg.HostName = hostname
		}
	}

	if cfg.proxy != nil {
		cfg.Transport.Proxy = cfg.proxy
	}

	// sanity check. This element is used with the modulo operator (%), so it can't be zero.
	// if it is, log the error, and assume the config was attempting to disable
	if cfg.Windows.ArgsRefreshInterval == 0 {
		log.Warnf("invalid configuration: windows_collect_skip_new_args was set to 0.  Disabling argument collection")
		cfg.Windows.ArgsRefreshInterval = -1
	}

	return cfg, nil
}

// NewNetworkAgentConfig returns a network-tracer specific AgentConfig using a configuration file. It can be nil
// if there is no file available. In this case we'll configure only via environment.
func NewNetworkAgentConfig(loggerName config.LoggerName, yamlPath string) (*AgentConfig, error) {
	cfg := NewDefaultAgentConfig()

	// When the network-tracer is enabled in a separate container, we need a way to also disable the network-tracer
	// packaged in the main agent container (without disabling network collection on the process-agent).
	//
	// If this environment flag is set, it'll sure it will not start
	if ok, _ := isAffirmative(os.Getenv("DD_NETWORK_TRACING_EXTERNAL")); ok {
		cfg.EnableNetworkTracing = false
		return cfg, nil
	}

	loadConfigIfExists(yamlPath)
	if err := cfg.loadNetworkYamlConfig(yamlPath); err != nil {
		return nil, err
	}

	// (Re)configure the logging from our configuration, with the network tracer log file + config options
	if err := setupLogger(loggerName, cfg.NetworkTracerLogFile, cfg); err != nil {
		log.Errorf("failed to setup configured logger: %s", err)
		return nil, err
	}

	return cfg, nil
}

func loadEnvVariables() {
	for envKey, cfgKey := range map[string]string{
		"DD_PROCESS_AGENT_ENABLED":          "process_config.enabled",
		"DD_PROCESS_AGENT_CONTAINER_SOURCE": "process_config.container_source",
		"DD_SCRUB_ARGS":                     "process_config.scrub_args",
		"DD_STRIP_PROCESS_ARGS":             "process_config.strip_proc_arguments",
		"DD_PROCESS_AGENT_URL":              "process_config.process_dd_url",

		// Note: this feature is in development and should not be used in production environments
		"DD_NETWORK_TRACING_ENABLED":  "network_tracer_config.enabled",
		"DD_NETTRACER_SOCKET":         "network_tracer_config.nettracer_socket",
		"DD_DISABLE_TCP_TRACING":      "network_tracer_config.disable_tcp",
		"DD_DISABLE_UDP_TRACING":      "network_tracer_config.disable_udp",
		"DD_DISABLE_IPV6_TRACING":     "network_tracer_config.disable_ipv6",
		"DD_COLLECT_LOCAL_DNS":        "network_tracer_config.collect_local_dns",
		"DD_USE_LOCAL_NETWORK_TRACER": "network_tracer_config.use_local_network_tracer",
		"DD_ENABLE_PROFILING":         "network_tracer_config.debug_profiling_enabled",

		"DD_DOGSTATSD_PORT": "dogstatsd_port",
		"DD_BIND_HOST":      "bind_host",
		"HTTPS_PROXY":       "proxy.https",
		"DD_PROXY_HTTPS":    "proxy.https",

		"DD_LOGS_STDOUT":    "log_to_console",
		"LOG_TO_CONSOLE":    "log_to_console",
		"DD_LOG_TO_CONSOLE": "log_to_console",
		"LOG_LEVEL":         "log_level", // Support LOG_LEVEL and DD_LOG_LEVEL but prefer DD_LOG_LEVEL
		"DD_LOG_LEVEL":      "log_level",
	} {
		if v, ok := os.LookupEnv(envKey); ok {
			config.Datadog.Set(cfgKey, v)
		}
	}

	// Support API_KEY and DD_API_KEY but prefer DD_API_KEY.
	if key, ddkey := os.Getenv("API_KEY"), os.Getenv("DD_API_KEY"); ddkey != "" {
		log.Info("overriding API key from env DD_API_KEY value")
		config.Datadog.Set("api_key", strings.TrimSpace(strings.Split(ddkey, ",")[0]))
	} else if key != "" {
		log.Info("overriding API key from env API_KEY value")
		config.Datadog.Set("api_key", strings.TrimSpace(strings.Split(key, ",")[0]))
	}

	if v := os.Getenv("DD_CUSTOM_SENSITIVE_WORDS"); v != "" {
		config.Datadog.Set("process_config.custom_sensitive_words", strings.Split(v, ","))
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

// SetupInitialLogger will set up a default logger before parsing config so we log errors nicely.
// The default will be stdout since we can't assume any file is writable.
func SetupInitialLogger(loggerName config.LoggerName) error {
	return config.SetupLogger(
		loggerName,
		"info",
		"",
		"",
		false,
		true, // logToConsole
		false,
	)
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
