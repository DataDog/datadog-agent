// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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
	APIKey                  string `mapstructure:"api_key" json:"api_key"`
	Host                    string
	Port                    int
	UseSSL                  *bool `mapstructure:"use_ssl" json:"use_ssl"`
	UseCompression          bool  `mapstructure:"use_compression" json:"use_compression"`
	CompressionLevel        int   `mapstructure:"compression_level" json:"compression_level"`
	ProxyAddress            string
	IsReliable              *bool `mapstructure:"is_reliable" json:"is_reliable"`
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

// GetUseSSL returns the UseSSL config setting
func (e *Endpoint) GetUseSSL() bool {
	return e.UseSSL == nil || *e.UseSSL
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
		if e.GetUseSSL() {
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
		if e.GetUseSSL() {
			protocol = "SSL encrypted TCP"
		} else {
			protocol = "TCP"
		}
	}

	return fmt.Sprintf("%sSending %s logs in %s to %s on port %d", prefix, compression, protocol, host, port)
}

// GetIsReliable returns true if the endpoint is reliable. Endpoints are reliable by default.
func (e *Endpoint) GetIsReliable() bool {
	return e.IsReliable == nil || *e.IsReliable
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
	return &Endpoints{
		Main:                   main,
		Endpoints:              append([]Endpoint{main}, additionalEndpoints...),
		UseProto:               useProto,
		UseHTTP:                useHTTP,
		BatchWait:              pkgconfigsetup.DefaultBatchWait,
		BatchMaxConcurrentSend: pkgconfigsetup.DefaultBatchMaxConcurrentSend,
		BatchMaxSize:           pkgconfigsetup.DefaultBatchMaxSize,
		BatchMaxContentSize:    pkgconfigsetup.DefaultBatchMaxContentSize,
		InputChanSize:          pkgconfigsetup.DefaultInputChanSize,
	}
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
		if endpoint.GetIsReliable() {
			endpoints = append(endpoints, endpoint)
		}
	}
	return endpoints
}

// GetUnReliableEndpoints returns additional endpoints that do not guarantee logs are received in the event of an error.
func (e *Endpoints) GetUnReliableEndpoints() []Endpoint {
	endpoints := []Endpoint{}
	for _, endpoint := range e.Endpoints {
		if !endpoint.GetIsReliable() {
			endpoints = append(endpoints, endpoint)
		}
	}
	return endpoints
}
