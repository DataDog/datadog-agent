package config

import (
	"bytes"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/legacy"
	"github.com/DataDog/datadog-agent/pkg/trace/flags"
	"github.com/DataDog/datadog-agent/pkg/trace/osutil"
	writerconfig "github.com/DataDog/datadog-agent/pkg/trace/writer/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// ErrMissingAPIKey is returned when the config could not be validated due to missing API key.
	ErrMissingAPIKey = errors.New("you must specify an API Key, either via a configuration file or the DD_API_KEY env var")

	// ErrMissingHostname is returned when the config could not be validated due to missing hostname.
	ErrMissingHostname = errors.New("failed to automatically set the hostname, you must specify it via configuration for or the DD_HOSTNAME env var")
)

// Endpoint specifies an endpoint that the trace agent will write data (traces, stats & services) to.
type Endpoint struct {
	APIKey string `json:"-"` // never marshal this
	Host   string

	// NoProxy will be set to true when the proxy setting for the trace API endpoint
	// needs to be ignored (e.g. it is part of the "no_proxy" list in the yaml settings).
	NoProxy bool
}

// AgentConfig handles the interpretation of the configuration (with default
// behaviors) in one place. It is also a simple structure to share across all
// the Agent components, with 100% safe and reliable values.
// It is exposed with expvar, so make sure to exclude any sensible field
// from JSON encoding. Use New() to create an instance.
type AgentConfig struct {
	Enabled bool

	// Global
	Hostname   string
	DefaultEnv string // the traces will default to this environment
	ConfigPath string // the source of this config, if any

	// Endpoints specifies the set of hosts and API keys where traces and stats
	// will be uploaded to. The first endpoint is the main configuration endpoint;
	// any following ones are read from the 'additional_endpoints' parts of the
	// configuration file, if present.
	Endpoints []*Endpoint

	// Concentrator
	BucketInterval   time.Duration // the size of our pre-aggregation per bucket
	ExtraAggregators []string

	// Sampler configuration
	ExtraSampleRate float64
	MaxTPS          float64
	MaxEPS          float64

	// Receiver
	ReceiverHost    string
	ReceiverPort    int
	ConnectionLimit int // for rate-limiting, how many unique connections to allow in a lease period (30s)
	ReceiverTimeout int

	// Writers
	ServiceWriterConfig writerconfig.ServiceWriterConfig
	StatsWriterConfig   writerconfig.StatsWriterConfig
	TraceWriterConfig   writerconfig.TraceWriterConfig

	// internal telemetry
	StatsdHost string
	StatsdPort int

	// logging
	LogLevel             string
	LogFilePath          string
	LogThrottlingEnabled bool

	// watchdog
	MaxMemory        float64       // MaxMemory is the threshold (bytes allocated) above which program panics and exits, to be restarted
	MaxCPU           float64       // MaxCPU is the max UserAvg CPU the program should consume
	MaxConnections   int           // (deprecated) MaxConnections is the threshold (opened TCP connections) above which program panics and exits, to be restarted
	WatchdogInterval time.Duration // WatchdogInterval is the delay between 2 watchdog checks

	// http/s proxying
	ProxyURL          *url.URL
	SkipSSLValidation bool

	// filtering
	Ignore map[string][]string

	// ReplaceTags is used to filter out sensitive information from tag values.
	// It maps tag keys to a set of replacements. Only supported in A6.
	ReplaceTags []*ReplaceRule

	// transaction analytics
	AnalyzedRateByServiceLegacy map[string]float64
	AnalyzedSpansByService      map[string]map[string]float64

	// infrastructure agent binary
	DDAgentBin string // DDAgentBin will be "" for Agent5 scenarios

	// Obfuscation holds sensitive data obufscator's configuration.
	Obfuscation *ObfuscationConfig
}

// New returns a configuration with the default values.
func New() *AgentConfig {
	return &AgentConfig{
		Enabled:    true,
		DefaultEnv: "none",
		Endpoints:  []*Endpoint{{Host: "https://trace.agent.datadoghq.com"}},

		BucketInterval:   time.Duration(10) * time.Second,
		ExtraAggregators: []string{"http.status_code"},

		ExtraSampleRate: 1.0,
		MaxTPS:          10,
		MaxEPS:          200,

		ReceiverHost:    "localhost",
		ReceiverPort:    8126,
		ConnectionLimit: 2000,

		ServiceWriterConfig: writerconfig.DefaultServiceWriterConfig(),
		StatsWriterConfig:   writerconfig.DefaultStatsWriterConfig(),
		TraceWriterConfig:   writerconfig.DefaultTraceWriterConfig(),

		StatsdHost: "localhost",
		StatsdPort: 8125,

		LogLevel:             "INFO",
		LogFilePath:          DefaultLogFilePath,
		LogThrottlingEnabled: true,

		MaxMemory:        5e8, // 500 Mb, should rarely go above 50 Mb
		MaxCPU:           0.5, // 50%, well behaving agents keep below 5%
		MaxConnections:   200, // in practice, rarely goes over 20
		WatchdogInterval: time.Minute,

		Ignore:                      make(map[string][]string),
		AnalyzedRateByServiceLegacy: make(map[string]float64),
		AnalyzedSpansByService:      make(map[string]map[string]float64),
	}
}

// Validate validates if the current configuration is good for the agent to start with.
func (c *AgentConfig) validate() error {
	if len(c.Endpoints) == 0 || c.Endpoints[0].APIKey == "" {
		return ErrMissingAPIKey
	}
	if c.Hostname == "" {
		if err := c.acquireHostname(); err != nil {
			return err
		}
	}
	return nil
}

// fallbackHostnameFunc specifies the function to use for obtaining the hostname
// when it can not be obtained by any other means. It is replaced in tests.
var fallbackHostnameFunc = os.Hostname

// acquireHostname attempts to acquire a hostname for this configuration. It
// tries to shell out to the infrastructure agent for this, if DD_AGENT_BIN is
// set, otherwise falling back to os.Hostname.
func (c *AgentConfig) acquireHostname() error {
	var cmd *exec.Cmd
	if c.DDAgentBin != "" {
		// Agent 6
		cmd = exec.Command(c.DDAgentBin, "hostname")
		cmd.Env = []string{}
	} else {
		// Most likely Agent 5. Try and obtain the hostname using the Agent's
		// Python environment, which will cover several additional installation
		// scenarios such as GCE, EC2, Kube, Docker, etc. In these scenarios
		// Go's os.Hostname will not be able to obtain the correct host. Do not
		// remove!
		cmd = exec.Command(defaultDDAgentPy, "-c", "from utils.hostname import get_hostname; print get_hostname()")
		cmd.Env = []string{defaultDDAgentPyEnv}
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Env = append(os.Environ(), cmd.Env...) // needed for Windows
	err := cmd.Run()
	c.Hostname = strings.TrimSpace(out.String())
	if err != nil || c.Hostname == "" {
		c.Hostname, err = fallbackHostnameFunc()
	}
	if c.Hostname == "" {
		err = ErrMissingHostname
	}
	return err
}

// Load returns a new configuration based on the given path. The path must not necessarily exist
// and a valid configuration can be returned based on defaults and environment variables. If a
// valid configuration can not be obtained, an error is returned.
func Load(path string) (*AgentConfig, error) {
	cfg, err := prepareConfig(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		log.Infof("Loaded configuration: %s", cfg.ConfigPath)
	}
	loadEnv() // TODO(gbbr): remove this along with all A5 configuration loading code and use BindEnv in pkg/config
	if err := config.ResolveSecrets(config.Datadog, filepath.Base(path)); err != nil {
		return cfg, err
	}
	cfg.applyDatadogConfig()
	return cfg, cfg.validate()
}

func prepareConfig(path string) (*AgentConfig, error) {
	cfgPath := path
	if cfgPath == flags.DefaultConfigPath && !osutil.Exists(cfgPath) && osutil.Exists(agent5Config) {
		// attempting to load inexistent default path, but found existing Agent 5
		// legacy config - try using it
		log.Warnf("Attempting to use Agent 5 configuration: %s", agent5Config)
		cfgPath = agent5Config
	}
	cfg := New()
	switch filepath.Ext(cfgPath) {
	case ".ini", ".conf":
		ac, err := legacy.GetAgentConfig(cfgPath)
		if err != nil {
			return cfg, err
		}
		if err := legacy.FromAgentConfig(ac); err != nil {
			return cfg, err
		}
	case ".yaml":
		cfg.DDAgentBin = defaultDDAgentBin
		config.Datadog.SetConfigFile(cfgPath)
		// we'll resolve secrets later, after loading environment variable values too
		if err := config.LoadWithoutSecret(); err != nil {
			return cfg, err
		}
	default:
		return cfg, errors.New("unrecognised file extension (need .yaml, .ini or .conf)")
	}
	cfg.ConfigPath = cfgPath
	return cfg, nil
}
