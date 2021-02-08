// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
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
	TargetTPS       float64
	MaxEPS          float64

	// Receiver
	ReceiverHost    string
	ReceiverPort    int
	ReceiverSocket  string // if not empty, UDS will be enabled on unix://<receiver_socket>
	ConnectionLimit int    // for rate-limiting, how many unique connections to allow in a lease period (30s)
	ReceiverTimeout int
	MaxRequestBytes int64 // specifies the maximum allowed request size for incoming trace payloads

	// Writers
	StatsWriter             *WriterConfig
	TraceWriter             *WriterConfig
	ConnectionResetInterval time.Duration // frequency at which outgoing connections are reset. 0 means no reset is performed

	// internal telemetry
	StatsdHost string
	StatsdPort int

	// logging
	LogLevel      string
	LogFilePath   string
	LogThrottling bool

	// watchdog
	MaxMemory        float64       // MaxMemory is the threshold (bytes allocated) above which program panics and exits, to be restarted
	MaxCPU           float64       // MaxCPU is the max UserAvg CPU the program should consume
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
	DDAgentBin string

	// Obfuscation holds sensitive data obufscator's configuration.
	Obfuscation *ObfuscationConfig

	// RequireTags represents a list of tags required for a root span.
	RequireTags []*Tag

	// RejectTags represents a list of tags that a root span must not have.
	RejectTags []*Tag
}

// Tag represents a key/value pair for filterable tags.
type Tag struct {
	K string
	V string
}

// New returns a configuration with the default values.
func New() *AgentConfig {
	return &AgentConfig{
		Enabled:    true,
		DefaultEnv: "none",
		Endpoints:  []*Endpoint{{Host: "https://trace.agent.datadoghq.com"}},

		BucketInterval: time.Duration(10) * time.Second,

		ExtraSampleRate: 1.0,
		TargetTPS:       10,
		MaxEPS:          200,

		ReceiverHost:    "localhost",
		ReceiverPort:    8126,
		MaxRequestBytes: 50 * 1024 * 1024, // 50MB

		StatsWriter:             new(WriterConfig),
		TraceWriter:             new(WriterConfig),
		ConnectionResetInterval: 0, // disabled

		StatsdHost: "localhost",
		StatsdPort: 8125,

		LogLevel:      "INFO",
		LogFilePath:   DefaultLogFilePath,
		LogThrottling: true,

		MaxMemory:        5e8, // 500 Mb, should rarely go above 50 Mb
		MaxCPU:           0.5, // 50%, well behaving agents keep below 5%
		WatchdogInterval: 10 * time.Second,

		Ignore:                      make(map[string][]string),
		AnalyzedRateByServiceLegacy: make(map[string]float64),
		AnalyzedSpansByService:      make(map[string]map[string]float64),

		DDAgentBin: defaultDDAgentBin,
	}
}

// APIKey returns the first (main) endpoint's API key.
func (c *AgentConfig) APIKey() string {
	if len(c.Endpoints) == 0 {
		return ""
	}
	return c.Endpoints[0].APIKey
}

// Validate validates if the current configuration is good for the agent to start with.
func (c *AgentConfig) validate() error {
	if len(c.Endpoints) == 0 || c.Endpoints[0].APIKey == "" {
		return ErrMissingAPIKey
	}
	if c.DDAgentBin == "" {
		return errors.New("agent binary path not set")
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
	var out bytes.Buffer
	cmd := exec.Command(c.DDAgentBin, "hostname")
	cmd.Env = append(os.Environ(), cmd.Env...) // needed for Windows
	cmd.Stdout = &out
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

// NewHTTPClient returns a new http.Client to be used for outgoing connections to the
// Datadog API.
func (c *AgentConfig) NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: c.NewHTTPTransport(),
	}
}

// NewHTTPTransport returns a new http.Transport to be used for outgoing connections to
// the Datadog API.
func (c *AgentConfig) NewHTTPTransport() *http.Transport {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: c.SkipSSLValidation},
		// below field values are from http.DefaultTransport (go1.12)
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if p := coreconfig.GetProxies(); p != nil {
		transport.Proxy = httputils.GetProxyTransportFunc(p)
	}
	return transport
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
	cfg.applyDatadogConfig()
	return cfg, cfg.validate()
}

func prepareConfig(path string) (*AgentConfig, error) {
	cfg := New()
	config.Datadog.SetConfigFile(path)
	if _, err := config.Load(); err != nil {
		return cfg, err
	}
	cfg.ConfigPath = path
	return cfg, nil
}

// HasFeature returns true if the feature f is present. Features are values
// of the DD_APM_FEATURES environment variable.
func HasFeature(f string) bool {
	return strings.Contains(os.Getenv("DD_APM_FEATURES"), f)
}
