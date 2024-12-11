// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkgconfigutils "github.com/DataDog/datadog-agent/pkg/config/utils"
)

// EPIntakeVersion is the events platform intake API version
type EPIntakeVersion uint8

// IntakeTrackType indicates the type of an endpoint intake.
type IntakeTrackType string

// IntakeProtocol indicates the protocol to use for an endpoint intake.
type IntakeProtocol string

// IntakeOrigin indicates the log source to use for an endpoint intake.
type IntakeOrigin string

const (
	_ EPIntakeVersion = iota
	// EPIntakeVersion1 is version 1 of the envets platform intake API
	EPIntakeVersion1
	// EPIntakeVersion2 is version 2 of the envets platform intake API
	EPIntakeVersion2
)

// Endpoint holds all the organization and network parameters to send logs to Datadog.
type Endpoint struct {
	apiKeyGetter func() string
	isReliable   bool
	useSSL       bool

	Host                    string `mapstructure:"host" json:"host"`
	Port                    int
	UseCompression          bool `mapstructure:"use_compression" json:"use_compression"`
	CompressionLevel        int  `mapstructure:"compression_level" json:"compression_level"`
	ProxyAddress            string
	IsMRF                   bool `mapstructure:"-" json:"-"`
	ConnectionResetInterval time.Duration

	BackoffFactor    float64
	BackoffBase      float64
	BackoffMax       float64
	RecoveryInterval int
	RecoveryReset    bool

	Version   EPIntakeVersion
	TrackType IntakeTrackType
	Protocol  IntakeProtocol
	Origin    IntakeOrigin
}

// unmarshalEndpoint is used to load additional endpoints from the configuration which stored as JSON/mapstructure.
// A different type is used than Endpoint since we want some fields to be private in Endpoint (APIKey, IsReliable, ...).
type unmarshalEndpoint struct {
	APIKey     string `mapstructure:"api_key" json:"api_key"`
	IsReliable *bool  `mapstructure:"is_reliable" json:"is_reliable"`
	UseSSL     *bool  `mapstructure:"use_ssl" json:"use_ssl"`

	Endpoint `mapstructure:",squash"`
}

// NewEndpoint returns a new Endpoint with the minimal field initialized.
func NewEndpoint(apiKey string, host string, port int, useSSL bool) Endpoint {
	apiKey = pkgconfigutils.SanitizeAPIKey(apiKey)
	return Endpoint{
		apiKeyGetter: func() string { return apiKey },
		Host:         host,
		Port:         port,
		useSSL:       useSSL,
		isReliable:   true, // by default endpoints are reliable
	}
}

// NewTCPEndpoint returns a new TCP Endpoint based on LogsConfigKeys. The endpoint is by default reliable and will use
// socks proxy and SSL settings from the configuration.
func NewTCPEndpoint(logsConfig *LogsConfigKeys) Endpoint {
	return Endpoint{
		apiKeyGetter:            logsConfig.getAPIKeyGetter(),
		ProxyAddress:            logsConfig.socks5ProxyAddress(),
		ConnectionResetInterval: logsConfig.connectionResetInterval(),
		useSSL:                  logsConfig.logsNoSSL(),
		isReliable:              true, // by default endpoints are reliable
	}
}

// NewHTTPEndpoint returns a new HTTP Endpoint based on LogsConfigKeys The endpoint is by default reliable and will use
// the settings related to HTTP from the configuration (compression, Backoff, recovery, ...).
func NewHTTPEndpoint(logsConfig *LogsConfigKeys) Endpoint {
	return Endpoint{
		apiKeyGetter:            logsConfig.getAPIKeyGetter(),
		UseCompression:          logsConfig.useCompression(),
		CompressionLevel:        logsConfig.compressionLevel(),
		ConnectionResetInterval: logsConfig.connectionResetInterval(),
		BackoffBase:             logsConfig.senderBackoffBase(),
		BackoffMax:              logsConfig.senderBackoffMax(),
		BackoffFactor:           logsConfig.senderBackoffFactor(),
		RecoveryInterval:        logsConfig.senderRecoveryInterval(),
		RecoveryReset:           logsConfig.senderRecoveryReset(),
		useSSL:                  logsConfig.logsNoSSL(),
		isReliable:              true, // by default endpoints are reliable
	}
}

// The setting from 'logs_config.additional_endpoints' is directly unmarshalled from the configuration into a
// []unmarshalEndpoint and do not use the constructors. In this case, apiKeyGetter is initialized to returned the API
// key from the loaded data instead of 'api_key'/'logs_config.api_key'.

func loadTCPAdditionalEndpoints(main Endpoint, l *LogsConfigKeys) []Endpoint {
	additionals := l.getAdditionalEndpoints()

	newEndpoints := make([]Endpoint, 0, len(additionals))
	for _, e := range additionals {
		newE := NewEndpoint(e.APIKey, e.Host, e.Port, false)

		newE.UseCompression = e.UseCompression
		newE.CompressionLevel = e.CompressionLevel
		newE.ProxyAddress = l.socks5ProxyAddress()
		newE.isReliable = e.IsReliable == nil || *e.IsReliable
		newE.ConnectionResetInterval = e.ConnectionResetInterval
		newE.BackoffFactor = e.BackoffFactor
		newE.BackoffBase = e.BackoffBase
		newE.BackoffMax = e.BackoffMax
		newE.RecoveryInterval = e.RecoveryInterval
		newE.RecoveryReset = e.RecoveryReset
		newE.Version = e.Version
		newE.TrackType = e.TrackType
		newE.Protocol = e.Protocol
		newE.Origin = e.Origin

		if e.UseSSL != nil {
			newE.useSSL = *e.UseSSL
		} else {
			newE.useSSL = main.useSSL
		}
		newEndpoints = append(newEndpoints, newE)
	}
	return newEndpoints
}

func loadHTTPAdditionalEndpoints(main Endpoint, l *LogsConfigKeys, intakeTrackType IntakeTrackType, intakeProtocol IntakeProtocol, intakeOrigin IntakeOrigin) []Endpoint {
	additionals := l.getAdditionalEndpoints()

	newEndpoints := make([]Endpoint, 0, len(additionals))
	for _, e := range additionals {
		newE := NewEndpoint(e.APIKey, e.Host, e.Port, false)

		newE.UseCompression = main.UseCompression
		newE.CompressionLevel = main.CompressionLevel
		newE.ProxyAddress = e.ProxyAddress
		newE.isReliable = e.IsReliable == nil || *e.IsReliable
		newE.ConnectionResetInterval = e.ConnectionResetInterval
		newE.BackoffFactor = main.BackoffFactor
		newE.BackoffBase = main.BackoffBase
		newE.BackoffMax = main.BackoffMax
		newE.RecoveryInterval = main.RecoveryInterval
		newE.RecoveryReset = main.RecoveryReset
		newE.Version = e.Version
		newE.TrackType = e.TrackType
		newE.Protocol = e.Protocol
		newE.Origin = e.Origin

		if e.UseSSL != nil {
			newE.useSSL = *e.UseSSL
		} else {
			newE.useSSL = main.useSSL
		}

		if newE.Version == 0 {
			newE.Version = main.Version
		}
		if newE.Version == EPIntakeVersion2 {
			newE.TrackType = intakeTrackType
			newE.Protocol = intakeProtocol
			newE.Origin = intakeOrigin
		}

		newEndpoints = append(newEndpoints, newE)
	}
	return newEndpoints
}

// GetAPIKey returns the latest API Key for the Endpoint, including when the configuration gets updated at runtime
func (e *Endpoint) GetAPIKey() string {
	return e.apiKeyGetter()
}

// UseSSL returns the useSSL config setting
func (e *Endpoint) UseSSL() bool {
	return e.useSSL
}

// GetStatus returns the endpoint status
func (e *Endpoint) GetStatus(prefix string, useHTTP bool) string {
	compression := "uncompressed"
	if e.UseCompression {
		compression = "compressed"
	}

	host := e.Host
	port := e.Port

	var protocol string
	if useHTTP {
		if e.UseSSL() {
			protocol = "HTTPS"
			if port == 0 {
				port = 443 // use default port
			}
		} else {
			protocol = "HTTP"
			// this case technically can't happens. In order to
			// disable SSL, user have to use a custom URL and
			// specify the port manually.
			if port == 0 {
				port = 80 // use default port
			}
		}
	} else {
		if e.UseSSL() {
			protocol = "SSL encrypted TCP"
		} else {
			protocol = "TCP"
		}
	}

	return fmt.Sprintf("%sSending %s logs in %s to %s on port %d", prefix, compression, protocol, host, port)
}

// IsReliable returns true if the endpoint is reliable. Endpoints are reliable by default.
func (e *Endpoint) IsReliable() bool {
	return e.isReliable
}

// Endpoints holds the main endpoint and additional ones to dualship logs.
type Endpoints struct {
	Main                   Endpoint
	Endpoints              []Endpoint
	UseProto               bool
	UseHTTP                bool
	BatchWait              time.Duration
	BatchMaxConcurrentSend int
	BatchMaxSize           int
	BatchMaxContentSize    int
	InputChanSize          int
}

// GetStatus returns the endpoints status, one line per endpoint
func (e *Endpoints) GetStatus() []string {
	result := make([]string, 0)
	for _, endpoint := range e.GetReliableEndpoints() {
		result = append(result, endpoint.GetStatus("Reliable: ", e.UseHTTP))
	}
	for _, endpoint := range e.GetUnReliableEndpoints() {
		result = append(result, endpoint.GetStatus("Unreliable: ", e.UseHTTP))
	}
	return result
}

// NewEndpoints returns a new endpoints composite with default batching settings
func NewEndpoints(main Endpoint, additionalEndpoints []Endpoint, useProto bool, useHTTP bool) *Endpoints {
	return NewEndpointsWithBatchSettings(
		main,
		additionalEndpoints,
		useProto,
		useHTTP,
		pkgconfigsetup.DefaultBatchWait,
		pkgconfigsetup.DefaultBatchMaxConcurrentSend,
		pkgconfigsetup.DefaultBatchMaxSize,
		pkgconfigsetup.DefaultBatchMaxContentSize,
		pkgconfigsetup.DefaultInputChanSize,
	)
}

// NewEndpointsWithBatchSettings returns a new endpoints composite with non-default batching settings specified
func NewEndpointsWithBatchSettings(main Endpoint, additionalEndpoints []Endpoint, useProto bool, useHTTP bool, batchWait time.Duration, batchMaxConcurrentSend int, batchMaxSize int, batchMaxContentSize int, inputChanSize int) *Endpoints {
	return &Endpoints{
		Main:                   main,
		Endpoints:              append([]Endpoint{main}, additionalEndpoints...),
		UseProto:               useProto,
		UseHTTP:                useHTTP,
		BatchWait:              batchWait,
		BatchMaxConcurrentSend: batchMaxConcurrentSend,
		BatchMaxSize:           batchMaxSize,
		BatchMaxContentSize:    batchMaxContentSize,
		InputChanSize:          inputChanSize,
	}
}

// GetReliableEndpoints returns additional endpoints that can be failed over to and block the pipeline in the
// event of an outage and will retry errors. These endpoints are treated the same as the main endpoint.
func (e *Endpoints) GetReliableEndpoints() []Endpoint {
	endpoints := []Endpoint{}
	for _, endpoint := range e.Endpoints {
		if endpoint.IsReliable() {
			endpoints = append(endpoints, endpoint)
		}
	}
	return endpoints
}

// GetUnReliableEndpoints returns additional endpoints that do not guarantee logs are received in the event of an error.
func (e *Endpoints) GetUnReliableEndpoints() []Endpoint {
	endpoints := []Endpoint{}
	for _, endpoint := range e.Endpoints {
		if !endpoint.IsReliable() {
			endpoints = append(endpoints, endpoint)
		}
	}
	return endpoints
}
