package config

import (
	"crypto/ecdsa"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
)

type Config struct {
	ActionsAllowlist  map[string][]string // map of allowed bundle IDs to a set of allowed action names
	Allowlist         []string
	AllowIMDSEndpoint bool
	DDHost            string
	Modes             []string
	OrgId             int64
	PrivateKey        *ecdsa.PrivateKey
	RunnerId          string
	Urn               string

	// RemoteConfig related fields
	DatadogSite string

	// the following are constants with default values. They are part of the config struct to allow for the ability to be overwritten in the YAML config file if needed
	MaxBackoff                time.Duration
	MinBackoff                time.Duration
	MaxAttempts               int32
	WaitBeforeRetry           time.Duration
	LoopInterval              time.Duration
	OpmsRequestTimeout        int32
	RunnerPoolSize            int32
	HealthCheckInterval       int32
	HttpServerReadTimeout     int32
	HttpServerWriteTimeout    int32
	RunnerAccessTokenHeader   string
	RunnerAccessTokenIdHeader string
	Port                      int32
	JWTRefreshInterval        time.Duration
	HealthCheckEndpoint       string
	HeartbeatInterval         time.Duration

	Version string

	MetricsClient statsd.ClientInterface
}
